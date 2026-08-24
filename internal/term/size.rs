//! Terminal-width probe.
//!
//! `table` reads the terminal width to size itself to fit. The Go original
//! uses `golang.org/x/term.GetSize(int(os.Stdout.Fd()))`. We mirror the call
//! via `ioctl(TIOCGWINSZ)` and fall back to 120 when the probe fails — same
//! constant the Go code uses as its fallback.

use std::os::unix::io::AsRawFd;

/// Width used when the probe cannot reach the controlling terminal
/// (CI runners, redirected output, piped stdin).
pub const FALLBACK_WIDTH: usize = 120;

/// Probe the current stdout width in columns.
///
/// Returns [`FALLBACK_WIDTH`] when stdout is not a terminal or the `ioctl`
/// fails for any reason.
#[must_use]
pub fn get_terminal_width() -> usize {
	let fd = std::io::stdout().as_raw_fd();
	get_terminal_width_at(fd).unwrap_or(FALLBACK_WIDTH)
}

/// Variant of [`get_terminal_width`] that targets an arbitrary file
/// descriptor. The box-drawing code uses `is_tty` to gate colour anyway, so
/// the only reason this exists is to give tests a single point at which to
/// inject a fake width — `isatty` cannot be mocked cleanly, but passing a
/// pipe fd here lets us exercise the fallback path explicitly.
#[must_use]
pub fn get_terminal_width_at(fd: std::os::unix::io::RawFd) -> Option<usize> {
	// `winsize` matches the kernel's `struct winsize` on Linux. The fields
	// are `ws_row`, `ws_col`, `ws_xpixel`, `ws_ypixel` — declared as
	// unsigned short by `<sys/ioctl.h>`.
	#[repr(C)]
	struct Winsize {
		ws_row: u16,
		ws_col: u16,
		ws_xpixel: u16,
		ws_ypixel: u16,
	}

	// The value is 0x5413 on every Linux architecture this ships for, but the
	// type is not portable: glibc's ioctl takes the request as c_ulong and
	// musl's takes c_int. The package builds for musl, so the constant has to
	// follow the target rather than assume glibc, as the previous comment on
	// this line did.
	#[cfg(target_env = "musl")]
	const TIOCGWINSZ: libc::c_int = 0x5413;
	#[cfg(not(target_env = "musl"))]
	const TIOCGWINSZ: libc::c_ulong = 0x5413;

	let mut ws = Winsize {
		ws_row: 0,
		ws_col: 0,
		ws_xpixel: 0,
		ws_ypixel: 0,
	};
	// SAFETY: `ws` is a stack-allocated POD of the right size; `ioctl`
	// writes into it and returns 0/-1. We check the return and only read
	// the field on success.
	let rc = unsafe { libc::ioctl(fd, TIOCGWINSZ, &mut ws) };
	if rc == 0 && ws.ws_col > 0 {
		Some(ws.ws_col as usize)
	} else {
		None
	}
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn fallback_when_stdout_is_not_a_tty() {
		// `cargo test` captures stdout via a pipe; the probe should return
		// the fallback rather than panic.
		let width = get_terminal_width();
		assert_eq!(width, FALLBACK_WIDTH);
	}

	#[test]
	fn returns_none_when_fd_is_not_a_tty() {
		// An arbitrary non-terminal fd (e.g. a pipe) yields None.
		let (read_end, _write_end) = std::os::unix::net::UnixStream::pair().expect("unix pair");
		assert_eq!(get_terminal_width_at(read_end.as_raw_fd()), None);
	}

	#[test]
	fn fallback_constant_is_one_twenty() {
		// Lock the fallback so a future change is a deliberate decision;
		// the table module sizes its columns against this number.
		assert_eq!(FALLBACK_WIDTH, 120);
	}
}
