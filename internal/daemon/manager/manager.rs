//! `Manager` — registry and daemon-wide rotation loop.
//!
//! Owns the `Arc<Mutex<HashMap<id, Process>>>` registry, the spec load
//! path, the scale / reload / delete ops, and the daemon-wide rotation
//! ticker. The `Process` struct itself lives in
//! [`crate::daemon::manager::process`] alongside its lifecycle methods;
//! this module is the registry and the operations on it.
//!
//! Mirrors `manager.go` for the top-level registry shape. The per-process
//! logic was extracted into its own module during the Phase-4b Rust port
//! — the Go tree keeps `Process` and `Manager` in the same 590-line file
//! because Go imports are cheap, but the Rust side splits by
//! responsibility.

use std::collections::{BTreeMap, HashMap};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::thread::JoinHandle;

use uuid::Uuid;

use crate::daemon::manager::helpers::{resolve_from_candidates, spawn_rotate_loop, ManagerError};
use crate::daemon::manager::process::Process;
use crate::daemon::manager::spawn::SpawnError;
use crate::ipc::protocol::{AppSpec, ScaleResponse};
use crate::spec;
use crate::types::ProcessInfo;

/// `UNITPM_MAX_PROCESSES` — parsed once at construction. Mirrors the Go
/// `Manager.maxProcesses` field.
pub const APP_LIMIT: &str = "UNITPM_MAX_PROCESSES";

/// `UNITPM_LOG_ROTATE_INTERVAL_MS` — rotation-tick cadence.
pub const ROTATE_TICK_ENV: &str = "UNITPM_LOG_ROTATE_INTERVAL_MS";

/// `UNITPM_TRIM_HEAP` — post-rotation `runtime.GC`.
pub const TRIM_HEAP_ENV: &str = "UNITPM_TRIM_HEAP";

// ManagerError lives in `helpers`; the manager module owns the registry
// type that produces it.

/// Snapshot of the registry used by [`Manager::scale`].
pub struct ScaleSnapshot {
	pub names: Vec<String>,
	pub ids: Vec<String>,
	pub template: Option<AppSpec>,
}

/// Top-level manager.
pub struct Manager {
	pub processes: HashMap<String, Arc<std::sync::Mutex<Process>>>,
	pub max_processes: usize,
	pub max_processes_err: Option<String>,
	pub rotate_stop: Arc<AtomicBool>,
	pub rotate_thread: Option<JoinHandle<()>>,
}

impl Manager {
	pub fn new() -> Self {
		let m = Self {
			processes: HashMap::new(),
			max_processes: 0,
			max_processes_err: None,
			rotate_stop: Arc::new(AtomicBool::new(false)),
			rotate_thread: None,
		};
		let (stop_flag, handle) = spawn_rotate_loop();
		let mut s = m;
		s.rotate_stop = stop_flag;
		s.rotate_thread = Some(handle);
		s
	}

	pub fn restore(&mut self) -> Result<(), ManagerError> {
		let specs = spec::load_all_protocol().map_err(|e| ManagerError::Restore(e.to_string()))?;
		for s in specs {
			if s.disabled {
				if let Err(e) = self.add_stopped_spec(s) {
					eprintln!("Error loading disabled process: {e}");
				}
				continue;
			}
			if let Err(e) = self.start_with_spec(s.clone()) {
				eprintln!("Error restoring process: {e}");
			}
		}
		Ok(())
	}

	pub fn add_stopped_spec(&mut self, s: AppSpec) -> Result<(), ManagerError> {
		let id = s.id.clone();
		let mut p = Process::new(&id, s).map_err(|e| ManagerError::Spawn(e.to_string()))?;
		p.no_auto_restart = true;
		p.stopped_by_user = true;
		self.processes
			.insert(id, Arc::new(std::sync::Mutex::new(p)));
		Ok(())
	}

	pub fn start_with_spec(&mut self, spec: AppSpec) -> Result<ProcessInfo, ManagerError> {
		if let Some(err) = &self.max_processes_err {
			return Err(ManagerError::Limits(err.clone()));
		}
		if self.max_processes > 0 && self.processes.len() >= self.max_processes {
			return Err(ManagerError::Limits("max processes reached".into()));
		}
		if self.processes.contains_key(&spec.id) {
			return Err(ManagerError::AlreadyExists(spec.id.clone()));
		}
		let id = spec.id.clone();
		let mut p =
			Process::new(&id, spec.clone()).map_err(|e| ManagerError::Spawn(e.to_string()))?;
		crate::daemon::manager::process::start_process(&mut p)
			.map_err(|e| ManagerError::Spawn(e.to_string()))?;
		if spec.disabled {
			let mut updated = spec.clone();
			updated.disabled = false;
			if let Err(e) = spec::save_spec_protocol(&updated.id, &updated) {
				eprintln!("Warning: failed to update spec for {}: {e}", updated.id);
			}
		}
		let info = p.info();
		self.processes
			.insert(id, Arc::new(std::sync::Mutex::new(p)));
		Ok(info)
	}

	pub fn get(&self, id: &str) -> Option<Arc<std::sync::Mutex<Process>>> {
		self.processes.get(id).cloned()
	}

	pub fn stop(&mut self, id: &str) -> Result<(), ManagerError> {
		let proc = self
			.processes
			.get(id)
			.cloned()
			.ok_or_else(|| ManagerError::ProcessNotFound(id.to_string()))?;
		{
			let mut p = proc.lock().unwrap_or_else(|e| e.into_inner());
			p.stop(true)
				.map_err(|e| ManagerError::Spawn(e.to_string()))?;
		}
		let mut updated = {
			let p = proc.lock().unwrap_or_else(|e| e.into_inner());
			p.spec_copy()
		};
		updated.disabled = true;
		if let Err(e) = spec::save_spec_protocol(&updated.id, &updated) {
			eprintln!("Warning: failed to save disabled state for {id}: {e}");
		}
		Ok(())
	}

	pub fn delete(&mut self, id: &str) -> Result<(), ManagerError> {
		let _ = self.stop(id);
		if self.processes.remove(id).is_none() {
			return Err(ManagerError::ProcessNotFound(id.to_string()));
		}
		Ok(())
	}

	pub fn restart(&mut self, id: &str) -> Result<(), ManagerError> {
		let proc = self
			.processes
			.get(id)
			.cloned()
			.ok_or_else(|| ManagerError::ProcessNotFound(id.to_string()))?;
		{
			let mut p = proc.lock().unwrap_or_else(|e| e.into_inner());
			p.reset_backoff();
		}
		{
			let mut p = proc.lock().unwrap_or_else(|e| e.into_inner());
			p.restart()
				.map_err(|e| ManagerError::Spawn(e.to_string()))?;
		}
		// Persist Disabled=false so the next daemon boot auto-auto-starts the spec.
		let mut p = proc.lock().unwrap_or_else(|e| e.into_inner());
		if p.spec.disabled {
			let mut updated = p.spec_copy();
			updated.disabled = false;
			if let Err(e) = spec::save_spec_protocol(&updated.id, &updated) {
				eprintln!("Warning: failed to clear Disabled flag for {id}: {e}");
			}
			p.spec.disabled = false;
		}
		Ok(())
	}

	pub fn reset(&self, id: &str) -> Result<(), ManagerError> {
		let proc = self
			.processes
			.get(id)
			.cloned()
			.ok_or_else(|| ManagerError::ProcessNotFound(id.to_string()))?;
		let mut p = proc.lock().unwrap_or_else(|e| e.into_inner());
		p.reset_metrics();
		Ok(())
	}

	pub fn list(&self) -> Vec<ProcessInfo> {
		let mut out = Vec::with_capacity(self.processes.len());
		for proc in self.processes.values() {
			let mut p = proc.lock().unwrap_or_else(|e| e.into_inner());
			out.push(p.info());
		}
		out
	}

	pub fn scale(
		&mut self,
		namespace: &str,
		base: &str,
		target: usize,
	) -> Result<ScaleResponse, ManagerError> {
		if target > 1024 {
			return Err(ManagerError::Limits("target count must be <= 1024".into()));
		}
		let ns = if namespace.is_empty() {
			"default".to_string()
		} else {
			namespace.to_string()
		};
		let snap = self.scale_snapshot(&ns, base);
		let mut res = ScaleResponse {
			base_name: base.to_string(),
			namespace: ns.clone(),
			before: snap.names.len() as i32,
			after: 0,
			created: None,
			deleted: None,
		};

		match target.cmp(&snap.names.len()) {
			std::cmp::Ordering::Equal => {
				res.after = target as i32;
				Ok(res)
			}
			std::cmp::Ordering::Less => {
				let to_remove = &snap.names[target..];
				for (i, _name) in to_remove.iter().enumerate().rev() {
					let id = &snap.ids[target + i];
					self.delete(id)
						.map_err(|e| ManagerError::Scale(e.to_string()))?;
				}
				let mut del = res.deleted.unwrap_or_default();
				del.extend(to_remove.iter().cloned());
				res.deleted = Some(del);
				res.after = target as i32;
				Ok(res)
			}
			std::cmp::Ordering::Greater => {
				if snap.names.is_empty() {
					return Err(ManagerError::NoTemplate(base.to_string()));
				}
				let template = snap.template.clone().unwrap();
				let mut taken: std::collections::HashSet<String> =
					snap.names.iter().cloned().collect();
				let mut next = 1usize;
				let mut created_names = Vec::new();
				for _ in 0..(target - snap.names.len()) {
					let mut candidate = format!("{base}-{next}");
					while taken.contains(&candidate) {
						next += 1;
						candidate = format!("{base}-{next}");
					}
					taken.insert(candidate.clone());
					next += 1;
					let mut new_spec = template.clone();
					new_spec.id = Uuid::now_v7().to_string();
					new_spec.name = candidate.clone();
					new_spec.namespace = Some(ns.clone());
					if new_spec.env.is_none() {
						new_spec.env = Some(BTreeMap::new());
					}
					if let Err(e) = spec::save_spec_protocol(&new_spec.id, &new_spec) {
						return Err(ManagerError::Scale(e.to_string()));
					}
					let failed_id = new_spec.id.clone();
					if let Err(e) = self.start_with_spec(new_spec) {
						let _ = spec::delete_spec_protocol(&failed_id);
						return Err(ManagerError::Scale(e.to_string()));
					}
					created_names.push(candidate);
				}
				res.created = Some(created_names);
				res.after = target as i32;
				Ok(res)
			}
		}
	}

	fn scale_snapshot(&self, namespace: &str, base: &str) -> ScaleSnapshot {
		let mut names: Vec<String> = Vec::new();
		let mut ids: Vec<String> = Vec::new();
		let mut template: Option<AppSpec> = None;
		let prefix = format!("{base}-");
		for (id, proc) in &self.processes {
			let p = proc.lock().unwrap_or_else(|e| e.into_inner());
			if p.info.namespace != namespace {
				continue;
			}
			if p.info.name == base {
				names.insert(0, p.info.name.clone());
				ids.insert(0, id.clone());
				continue;
			}
			if let Some(rest) = p.info.name.strip_prefix(&prefix) {
				if let Ok(n) = rest.parse::<u32>() {
					names.push(p.info.name.clone());
					ids.push(id.clone());
					let _ = n;
				}
			}
		}
		if let Some(first_id) = ids.first() {
			if let Some(p) = self.processes.get(first_id) {
				let p = p.lock().unwrap_or_else(|e| e.into_inner());
				template = Some(p.spec_copy());
			}
		}
		ScaleSnapshot {
			names,
			ids,
			template,
		}
	}

	pub fn reload(&mut self, id: &str) -> Result<(), ManagerError> {
		let mut s =
			spec::load_spec_protocol(id).map_err(|e| ManagerError::Reload(e.to_string()))?;
		if s.namespace.as_deref().unwrap_or("").is_empty() {
			s.namespace = Some("default".into());
		}
		s.disabled = false;
		if let Err(e) = spec::save_spec_protocol(&s.id, &s) {
			eprintln!("Warning: failed to save spec for {}: {e}", s.id);
		}
		// Stop the old process and start a new one.
		let _ = self.stop(id);
		if let Some(p) = self.processes.remove(id) {
			let _ = p; // drop
		}
		let info = self
			.start_with_spec(s.clone())
			.map_err(|e| ManagerError::Reload(e.to_string()))?;
		let _ = info;
		Ok(())
	}

	pub fn resolve_id(&self, identifier: &str) -> Result<String, ManagerError> {
		if let Some((ns, name)) = identifier.split_once(':') {
			let mut candidates: Vec<String> = Vec::new();
			for (id, proc) in &self.processes {
				let p = proc.lock().unwrap_or_else(|e| e.into_inner());
				if p.info.namespace == ns && p.info.name == name {
					candidates.push(id.clone());
				}
			}
			return resolve_from_candidates(identifier, &candidates);
		}
		if self.processes.contains_key(identifier) {
			return Ok(identifier.to_string());
		}
		let mut candidates: Vec<String> = Vec::new();
		for id in self.processes.keys() {
			if id.starts_with(identifier) {
				candidates.push(id.clone());
			}
		}
		if !candidates.is_empty() {
			return resolve_from_candidates(identifier, &candidates);
		}
		for (id, proc) in &self.processes {
			let p = proc.lock().unwrap_or_else(|e| e.into_inner());
			if p.info.name == identifier {
				candidates.push(id.clone());
			}
		}
		resolve_from_candidates(identifier, &candidates)
	}

	pub fn shutdown(&mut self) {
		self.rotate_stop.store(true, Ordering::Relaxed);
		if let Some(h) = self.rotate_thread.take() {
			let _ = h.join();
		}
		let procs: Vec<Arc<std::sync::Mutex<Process>>> = self.processes.values().cloned().collect();
		for p in procs {
			let mut p = p.lock().unwrap_or_else(|e| e.into_inner());
			let _ = p.stop(false);
		}
	}
}

impl Default for Manager {
	fn default() -> Self {
		Self::new()
	}
}

fn _bind_process(s: &SpawnError) -> String {
	format!("{s}")
}
