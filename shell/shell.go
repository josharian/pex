package shell

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"unicode"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

var parser = syntax.NewParser(syntax.Variant(syntax.LangPOSIX), syntax.KeepComments(true))

type Command struct {
	Argv []string
	Raw  string
}

func Parse(s string) ([]Command, []int, error) {
	slog.Debug("shell.Parse", "rawInput", s)
	orig := s
	trimEnd := strings.TrimRightFunc(s, unicode.IsSpace)
	if strings.HasSuffix(trimEnd, "|") && !strings.HasSuffix(trimEnd, "||") {
		s = strings.TrimSuffix(trimEnd, "|")
	}
	var trailing Command
	var trailingPipe int
	var hasTrailing bool
	if orig != s {
		trailing = Command{
			Argv: nil,
			Raw:  orig[len(trimEnd):],
		}
		trailingPipe = len(trimEnd) - 1
		hasTrailing = true
	}
	f, err := parser.Parse(strings.NewReader(s), "")
	if err != nil {
		return nil, nil, err
	}
	if len(f.Stmts) == 0 {
		if hasTrailing {
			return nil, nil, fmt.Errorf("1:%d: missing statement before |", trailingPipe)
		}
		return nil, nil, nil
	}
	// First syntax pass: Eliminate anything verboten.
	// Be very conservative for now, using an allowlist.
	syntax.Walk(f, func(n syntax.Node) bool {
		switch n := n.(type) {
		case nil, *syntax.File, *syntax.CallExpr, *syntax.Word,
			*syntax.Lit, *syntax.SglQuoted, *syntax.DblQuoted:
		case *syntax.BinaryCmd:
			// TODO: consider supporting |& (syntax.PipeAll) to pipe stderr as well
			if n.Op != syntax.Pipe {
				err = fmt.Errorf("%s is not supported", n.Op.String())
			}
		case *syntax.Stmt:
			if n.Negated || n.Background || n.Coprocess {
				err = fmt.Errorf("negated or background commands are not supported")
			}
		default:
			err = errors.New(notSupported(n)) // all other nodes
		}
		return err == nil
	})
	if err != nil {
		return nil, nil, err
	}
	// Second pass: debug print.
	if slog.Default().Enabled(nil, slog.LevelDebug) {
		buf := new(strings.Builder)
		indent := 0
		syntax.Walk(f, func(n syntax.Node) bool {
			if n == nil {
				indent--
			} else {
				fmt.Fprint(buf, strings.Repeat("  ", indent))
				fmt.Fprintf(buf, "%T %v\n", n, n)
				indent++
			}
			return true
		})
		fmt.Fprintln(buf)
		slog.Debug("shell.Parse AST", "tree", buf.String())
	}
	// Third pass: extract.
	var commands []Command
	var pipes []int
	syntax.Walk(f, func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.BinaryCmd:
			if n.Op == syntax.Pipe {
				pipes = append(pipes, int(n.OpPos.Offset()))
			}
		case *syntax.CallExpr:
			if len(n.Assigns) > 0 {
				err = fmt.Errorf("variables are not supported")
				return false
			}
			cmd := Command{
				Raw: s[n.Pos().Offset():n.End().Offset()],
			}
			cmd.Argv, err = expand.Fields(nil, n.Args...)
			if err != nil {
				return false
			}
			commands = append(commands, cmd)
		}
		return true
	})
	if hasTrailing {
		commands = append(commands, trailing)
		pipes = append(pipes, trailingPipe)
	}
	sort.Slice(pipes, func(i, j int) bool { return pipes[i] < pipes[j] })
	return commands, pipes, nil
}

func (p Command) Equal(q Command) bool {
	return slices.Equal(p.Argv, q.Argv)
}

func (p Command) Empty() bool {
	return len(p.Argv) == 0
}

func (p Command) Name() string {
	if p.Empty() {
		return ""
	}
	return p.Argv[0]
}

func (p Command) Args() []string {
	if p.Empty() {
		return nil
	}
	return p.Argv[1:]
}

func notSupported(n syntax.Node) string {
	switch n := n.(type) {
	case *syntax.Redirect:
		return "redirects are not supported"
	case *syntax.IfClause:
		return "if clauses are not supported"
	case *syntax.ForClause:
		return "for clauses are not supported"
	case *syntax.Block:
		return "blocks are not supported"
	case *syntax.Assign:
		return "variables are not supported"
	case *syntax.ProcSubst:
		return "process substitution is not supported"
	case *syntax.Subshell:
		return "subshells are not supported"
	case *syntax.ParamExp:
		return "parameter expansion is not supported"
	case *syntax.CmdSubst:
		return "command substitution is not supported"
	case *syntax.ArithmExp, *syntax.ArithmCmd:
		// TODO: consider supporting ArithmExp, the expand package handles it.
		return "arithmetic expressions are not supported"
	case *syntax.Comment:
		// comments make it harder to handle trailing pipes
		return "comments are not supported"
	default: // some fallback; add more human-friendly cases above as needed
		return fmt.Sprintf("%T nodes are not supported", n)
	}
}
