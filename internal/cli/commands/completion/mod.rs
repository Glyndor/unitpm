//! The `completion` command.
//!
//! 8 cases ported from `internal/cli/commands/completion/cmd_test.go`.
//!
//! Emits a shell completion script for bash, zsh, or fish. The visible
//! command list is read from the registry, so a registration that adds a
//! new command automatically appears in the generated scripts.

use std::io::Write;

use crate::cli::help::CommandSpec;
use crate::cli::registry;

/// Run the `completion` command. `w` receives the script body.
pub fn run<W: Write>(w: &mut W, args: &[String]) -> Result<(), Box<dyn std::error::Error>> {
	if args.iter().any(|a| a == "-h" || a == "--help") {
		print_help(w);
		return Ok(());
	}

	if args.is_empty() {
		return Err(usage(
			"usage: unitpm completion <bash|zsh|fish>".to_string(),
		));
	}

	let shell = &args[0];
	match shell.as_str() {
		"bash" => write_bash(w),
		"zsh" => write_zsh(w),
		"fish" => write_fish(w),
		other => Err(unsupported_shell(other)),
	}
}

fn unsupported_shell(s: &str) -> Box<dyn std::error::Error> {
	Box::<dyn std::error::Error>::from(format!(
		"unsupported shell {s:?} (supported: bash, zsh, fish)"
	))
}

fn usage(msg: String) -> Box<dyn std::error::Error> {
	Box::<dyn std::error::Error>::from(msg)
}

/// Visible (non-hidden) command names + aliases, sorted.
fn visible_commands() -> Vec<String> {
	let specs = registry::get_all();
	let mut out: Vec<String> = Vec::new();
	for s in &specs {
		if s.hidden {
			continue;
		}
		out.push(s.name.clone());
		out.extend(s.aliases.iter().cloned());
	}
	out.sort();
	out
}

fn write_bash<W: Write>(w: &mut W) -> Result<(), Box<dyn std::error::Error>> {
	let cmds = visible_commands().join(" ");
	let script = format!(
		r#"# bash completion for unitpm
_unitpm_completions() {{
    local cur prev words cword
    _init_completion -n : || return

    local cmds="{cmds}"

    if [[ ${{cword}} -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "${{cmds}}" -- "${{cur}}") )
        return
    fi

    case "${{words[1]}}" in
        stop|restart|reload|flush|delete|rm|remove|show|logs|log)
            # Second arg: complete process names from 'unitpm list'
            local names
            names=$(unitpm list --long 2>/dev/null | awk -F'|' 'NR>3 {{gsub(/ /,"",$3); if ($3!="") print $3}}')
            COMPREPLY=( $(compgen -W "${{names}}" -- "${{cur}}") )
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish" -- "${{cur}}") )
            ;;
        help)
            COMPREPLY=( $(compgen -W "${{cmds}}" -- "${{cur}}") )
            ;;
    esac
}}
complete -F _unitpm_completions unitpm
"#
	);
	w.write_all(script.as_bytes())?;
	Ok(())
}

fn write_zsh<W: Write>(w: &mut W) -> Result<(), Box<dyn std::error::Error>> {
	let cmds = visible_commands().join(" ");
	let script = format!(
		r#"#compdef unitpm
# zsh completion for unitpm

_unitpm() {{
    local -a cmds
    cmds=({cmds})

    if (( CURRENT == 2 )); then
        _describe 'command' cmds
        return
    fi

    case "${{words[2]}}" in
        stop|restart|reload|flush|delete|rm|remove|show|logs|log)
            local -a names
            names=( ${{(f)"$(unitpm list --long 2>/dev/null | awk -F'|' 'NR>3 {{gsub(/ /,"",$3); if ($3!="") print $3}}')"}} )
            _describe 'process' names
            ;;
        completion)
            _values 'shell' bash zsh fish
            ;;
        help)
            _describe 'command' cmds
            ;;
    esac
}}
_unitpm "$@"
"#
	);
	w.write_all(script.as_bytes())?;
	Ok(())
}

fn write_fish<W: Write>(w: &mut W) -> Result<(), Box<dyn std::error::Error>> {
	let mut b = String::new();
	b.push_str("# fish completion for unitpm\n\n");

	for s in registry::get_all() {
		if s.hidden {
			continue;
		}
		let desc = s.description.replace('\'', "");
		b.push_str(&format!(
			"complete -c unitpm -n '__fish_use_subcommand' -a '{}' -d '{}'\n",
			s.name, desc
		));
		for alias in &s.aliases {
			b.push_str(&format!(
				"complete -c unitpm -n '__fish_use_subcommand' -a '{}' -d 'Alias for {}'\n",
				alias, s.name
			));
		}
	}

	b.push_str(
		r#"
function __unitpm_list_names
    unitpm list --long 2>/dev/null | awk -F'|' 'NR>3 {gsub(/ /,"",$3); if ($3!="") print $3}'
end

for cmd in stop restart reload flush delete rm remove show logs log
    complete -c unitpm -n "__fish_seen_subcommand_from $cmd" -f -a "(__unitpm_list_names)"
end

complete -c unitpm -n '__fish_seen_subcommand_from completion' -f -a 'bash zsh fish'
"#,
	);
	w.write_all(b.as_bytes())?;
	Ok(())
}

/// Help block for `--help`.
pub fn print_help<W: Write>(w: &mut W) {
	let _ = crate::cli::help::render_command_help(w, &spec());
}

/// Spec used by the registry / help renderer.
#[must_use]
pub fn spec() -> CommandSpec {
	CommandSpec {
		name: "completion".to_string(),
		aliases: Vec::new(),
		usage: "unitpm completion <bash|zsh|fish>".to_string(),
		description: "Generate a shell completion script.".to_string(),
		options: vec![crate::cli::help::Option {
			short: "-h".to_string(),
			long: "--help".to_string(),
			description: "Show this help message.".to_string(),
		}],
		examples: vec![
			"# Bash".to_string(),
			"  unitpm completion bash > ~/.local/share/bash-completion/completions/unitpm"
				.to_string(),
			"# Zsh (writes to a dir in $fpath)".to_string(),
			"  unitpm completion zsh > ${fpath[1]}/_unitpm".to_string(),
			"# Fish".to_string(),
			"  unitpm completion fish > ~/.config/fish/completions/unitpm.fish".to_string(),
		],
		hidden: false,
	}
}

#[cfg(test)]
mod tests;
