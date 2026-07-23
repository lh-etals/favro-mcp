package install

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// choice represents one selectable client row.
type choice struct {
	id      string
	label   string
	checked bool
}

// multiSelect shows an interactive checkbox list and returns the IDs of the
// checked rows. Falls back to a plain numbered prompt when stdin is not a TTY
// or raw mode is unavailable.
func multiSelect(prompt string, choices []choice) ([]string, error) {
	if len(choices) == 0 {
		return nil, nil
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fallbackSelect(prompt, choices), nil
	}

	old, err := term.MakeRaw(fd)
	if err != nil {
		return fallbackSelect(prompt, choices), nil
	}
	defer term.Restore(fd, old)

	cursor := 0
	for {
		var b strings.Builder
		b.WriteString("\r\033[K")
		b.WriteString(prompt)
		b.WriteString("  (↑/↓ move, space toggle, enter confirm)\n")
		for i, c := range choices {
			marker := " "
			if c.checked {
				marker = "x"
			}
			line := fmt.Sprintf("  [%s] %s", marker, c.label)
			if i == cursor {
				line = "\033[36m> " + line[2:] + "\033[0m"
			}
			b.WriteString("\r\033[K" + line + "\n")
		}
		b.WriteString("\r\033[K")
		os.Stdout.WriteString(b.String())

		buf := make([]byte, 3)
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return nil, err
		}
		seq := buf[:n]
		switch {
		case seq[0] == 0x1b && n >= 2 && seq[1] == '[' && n >= 3 && seq[2] == 'A': // up
			if cursor > 0 {
				cursor--
			}
		case seq[0] == 0x1b && n >= 2 && seq[1] == '[' && n >= 3 && seq[2] == 'B': // down
			if cursor < len(choices)-1 {
				cursor++
			}
		case seq[0] == ' ': // toggle
			choices[cursor].checked = !choices[cursor].checked
		case seq[0] == '\r' || seq[0] == '\n': // confirm
			os.Stdout.WriteString("\r\033[K\033[" + fmt.Sprintf("%d", len(choices)+1) + "A")
			var out []string
			for _, c := range choices {
				if c.checked {
					out = append(out, c.id)
				}
			}
			return out, nil
		case seq[0] == 3 || seq[0] == 'q': // ctrl-c / q
			os.Stdout.WriteString("\r\033[K")
			return nil, fmt.Errorf("cancelled")
		}
	}
}

// fallbackSelect is a non-TTY, line-based prompt (detected rows pre-checked).
func fallbackSelect(prompt string, choices []choice) []string {
	fmt.Println(prompt)
	for i, c := range choices {
		mark := " "
		if c.checked {
			mark = "*"
		}
		fmt.Printf("  %s %2d. %s\n", mark, i+1, c.label)
	}
	fmt.Print("Enter comma-separated numbers (blank = keep * rows): ")
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	if strings.TrimSpace(line) == "" {
		var out []string
		for _, c := range choices {
			if c.checked {
				out = append(out, c.id)
			}
		}
		return out
	}
	idx := map[int]bool{}
	for _, p := range strings.Split(line, ",") {
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(p), "%d", &n); err == nil {
			idx[n-1] = true
		}
	}
	var out []string
	for i, c := range choices {
		if idx[i] {
			out = append(out, c.id)
		}
	}
	return out
}
