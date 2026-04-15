// Package completion emits shell-completion scripts for bash, zsh, and fish.
package completion

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/cli/registry"
)

// Run writes a completion script for the requested shell to stdout.
func Run(args []string) error {
	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: lynx completion <bash|zsh|fish>")
	}

	shell := args[0]
	switch shell {
	case "bash":
		return writeBash(os.Stdout)
	case "zsh":
		return writeZsh(os.Stdout)
	case "fish":
		return writeFish(os.Stdout)
	default:
		return fmt.Errorf("unsupported shell %q (supported: bash, zsh, fish)", shell)
	}
}

// visibleCommands returns command names + aliases, excluding hidden internals.
func visibleCommands() []string {
	specs := registry.GetAll()
	out := []string{}
	for _, s := range specs {
		if s.Hidden {
			continue
		}
		out = append(out, s.Name)
		out = append(out, s.Aliases...)
	}
	sort.Strings(out)
	return out
}

func writeBash(w io.Writer) error {
	cmds := strings.Join(visibleCommands(), " ")
	script := `# bash completion for lynx
_lynx_completions() {
    local cur prev words cword
    _init_completion -n : || return

    local cmds="` + cmds + `"

    if [[ ${cword} -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "${cmds}" -- "${cur}") )
        return
    fi

    case "${words[1]}" in
        stop|restart|reload|flush|delete|rm|remove|show|logs|log)
            # Second arg: complete process names from 'lynx list'
            local names
            names=$(lynx list --long 2>/dev/null | awk -F'|' 'NR>3 {gsub(/ /,"",$3); if ($3!="") print $3}')
            COMPREPLY=( $(compgen -W "${names}" -- "${cur}") )
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish" -- "${cur}") )
            ;;
        help)
            COMPREPLY=( $(compgen -W "${cmds}" -- "${cur}") )
            ;;
    esac
}
complete -F _lynx_completions lynx
`
	_, err := w.Write([]byte(script))
	return err
}

func writeZsh(w io.Writer) error {
	cmds := strings.Join(visibleCommands(), " ")
	script := `#compdef lynx
# zsh completion for lynx

_lynx() {
    local -a cmds
    cmds=(` + cmds + `)

    if (( CURRENT == 2 )); then
        _describe 'command' cmds
        return
    fi

    case "${words[2]}" in
        stop|restart|reload|flush|delete|rm|remove|show|logs|log)
            local -a names
            names=( ${(f)"$(lynx list --long 2>/dev/null | awk -F'|' 'NR>3 {gsub(/ /,"",$3); if ($3!="") print $3}')"} )
            _describe 'process' names
            ;;
        completion)
            _values 'shell' bash zsh fish
            ;;
        help)
            _describe 'command' cmds
            ;;
    esac
}
_lynx "$@"
`
	_, err := w.Write([]byte(script))
	return err
}

func writeFish(w io.Writer) error {
	var b strings.Builder
	b.WriteString("# fish completion for lynx\n\n")

	// Only top-level command completions for fish (lists are fetched lazily).
	for _, s := range registry.GetAll() {
		if s.Hidden {
			continue
		}
		desc := strings.ReplaceAll(s.Description, "'", "")
		fmt.Fprintf(&b,
			"complete -c lynx -n '__fish_use_subcommand' -a %q -d %q\n",
			s.Name, desc,
		)
		for _, alias := range s.Aliases {
			fmt.Fprintf(&b,
				"complete -c lynx -n '__fish_use_subcommand' -a %q -d %q\n",
				alias, "Alias for "+s.Name,
			)
		}
	}

	// Process-name completion for commands that target running apps.
	b.WriteString(`
function __lynx_list_names
    lynx list --long 2>/dev/null | awk -F'|' 'NR>3 {gsub(/ /,"",$3); if ($3!="") print $3}'
end

for cmd in stop restart reload flush delete rm remove show logs log
    complete -c lynx -n "__fish_seen_subcommand_from $cmd" -f -a "(__lynx_list_names)"
end

complete -c lynx -n '__fish_seen_subcommand_from completion' -f -a 'bash zsh fish'
`)
	_, err := w.Write([]byte(b.String()))
	return err
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "completion",
		Usage:       "lynx completion <bash|zsh|fish>",
		Description: "Generate a shell completion script.",
		Examples: []string{
			"# Bash",
			"  lynx completion bash > ~/.local/share/bash-completion/completions/lynx",
			"# Zsh (writes to a dir in $fpath)",
			"  lynx completion zsh > ${fpath[1]}/_lynx",
			"# Fish",
			"  lynx completion fish > ~/.config/fish/completions/lynx.fish",
		},
	}
}

// PrintHelp prints the help message.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
