package build

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Parser reads a Docksmithfile and routes commands to an executor
type Parser struct {
	executor *Executor
}

// NewParser creates a new Parser linked to an executor
func NewParser(executor *Executor) *Parser {
	return &Parser{executor: executor}
}

// Parse reads the file line by line executing instructions sequentially
func (p *Parser) Parse(reader io.Reader) error {
	// 1. Slurp all lines to prep counters
	var lines []string
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	// 2. Count total instructions
	totalSteps := 0
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			totalSteps++
		}
	}
	p.executor.state.StepTotal = totalSteps
	p.executor.state.StepCurrent = 0
	p.executor.state.CurrentLine = 0

	// 3. Execute instructions
	for _, line := range lines {
		p.executor.state.CurrentLine++
		trimmed := strings.TrimSpace(line)

		// Ignore empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		parts := strings.SplitN(trimmed, " ", 2)
		instruction := strings.ToUpper(parts[0])

		var arg string
		if len(parts) > 1 {
			arg = strings.TrimSpace(parts[1])
		}

		if err := p.executeInstruction(instruction, arg); err != nil {
			return fmt.Errorf("[Error] line %d: %w", p.executor.state.CurrentLine, err)
		}
	}
	return nil
}

	// Basic validation at end of file
	if p.executor.state.BaseImage == "" && p.executor.state.CurrentLine > 0 {
		return fmt.Errorf("Parse failed: no FROM instruction provided")
	}

	return nil
}

func (p *Parser) executeInstruction(instruction, arg string) error {
	switch instruction {
	case "FROM":
		if arg == "" {
			return fmt.Errorf("FROM requires an image argument")
		}
		return p.executor.EvalFROM(arg)
		
	case "WORKDIR":
		if arg == "" {
			return fmt.Errorf("WORKDIR requires a path argument")
		}
		return p.executor.EvalWORKDIR(arg)
		
	case "ENV":
		if arg == "" || !strings.Contains(arg, "=") {
			return fmt.Errorf("ENV requires a KEY=VALUE argument")
		}
		return p.executor.EvalENV(arg)
		
	case "CMD":
		if arg == "" {
			return fmt.Errorf("CMD requires a JSON array argument")
		}
		return p.executor.EvalCMD(arg)
		
	case "COPY":
		// Standard COPY <src> <dest> requires 2 arguments.
		// We use Fields to handle arbitrary whitespace between arguments.
		f := strings.Fields(arg)
		if len(f) < 2 {
			return fmt.Errorf("COPY requires source and destination arguments")
		}
		// In Docksmith, we treat the first part as SRC and the second as DEST.
		// If DEST also has spaces, the user should be using JSON (not yet supported)
		// or we can join the rest. Let's join the rest for flexibility.
		src := f[0]
		dest := strings.Join(f[1:], " ")
		return p.executor.EvalCOPY(src, dest)
		
	case "RUN":
		if arg == "" {
			return fmt.Errorf("RUN requires a command argument")
		}
		return p.executor.EvalRUN(arg)
		
	default:
		return fmt.Errorf("Unknown instruction '%s'", instruction)
	}
}
