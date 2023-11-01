package shell

import (
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"unicode"

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
	// Be very conservative for now.
	syntax.Walk(f, func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.IfClause:
			err = fmt.Errorf("if statements are not supported")
		case *syntax.Block:
			err = fmt.Errorf("shell blocks are not supported")
		case *syntax.Redirect:
			err = fmt.Errorf("redirects are not supported")
		case *syntax.CallExpr:
			if len(n.Assigns) > 0 {
				err = fmt.Errorf("variables are not supported")
			}
		case *syntax.ProcSubst:
			err = fmt.Errorf("process substitution is not supported")
		case *syntax.DblQuoted:
			if n.Dollar {
				err = fmt.Errorf("dollar in double quotes is not supported")
			}
		case *syntax.Subshell:
			err = fmt.Errorf("subshells are not supported")
		case *syntax.ParamExp:
			err = fmt.Errorf("parameter expansion is not supported")
		case *syntax.CmdSubst:
			err = fmt.Errorf("command substitution is not supported")
		case *syntax.ArithmExp:
			err = fmt.Errorf("arithmetic expansion is not supported")
		case *syntax.ExtGlob:
			err = fmt.Errorf("extended globs are not supported")
		case *syntax.Comment:
			// comments make it harder to handle trailing pipes
			err = fmt.Errorf("comments are not supported")
		case *syntax.BinaryArithm:
			err = fmt.Errorf("binary arithmetic is not supported")
		case *syntax.BinaryCmd:
			switch n.Op {
			case syntax.AndStmt:
				err = fmt.Errorf("&& is not supported")
			case syntax.OrStmt:
				err = fmt.Errorf("|| is not supported")
			}
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
			}
			var args []string
			for _, word := range n.Args {
				args = append(args, wordToArg(word.Parts))
			}
			commands = append(commands, Command{
				Argv: args,
				Raw:  s[n.Pos().Offset():n.End().Offset()],
			})
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

func wordToArg(parts []syntax.WordPart) string {
	var x []string
	for _, part := range parts {
		switch part := part.(type) {
		case *syntax.Lit:
			x = append(x, part.Value)
		case *syntax.SglQuoted:
			x = append(x, part.Value)
		case *syntax.DblQuoted:
			x = append(x, wordToArg(part.Parts))
		default:
			panic(fmt.Sprintf("unsupported word part: %T", part))
		}
	}
	return strings.Join(x, "")
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
