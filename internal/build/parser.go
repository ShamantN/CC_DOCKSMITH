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
	scanner := bufio.NewScanner(reader)
	p.executor.state.CurrentLine = 0

	for scanner.Scan() {
		p.executor.state.CurrentLine++
		line := strings.TrimSpace(scanner.Text())

		// Ignore empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		instruction := strings.ToUpper(parts[0])

		var arg string
		if len(parts) > 1 {
			arg = strings.TrimSpace(parts[1])
		}

		if err := p.executeInstruction(instruction, arg); err != nil {
			return fmt.Errorf("[Error] line %d: %w", p.executor.state.CurrentLine, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading file: %w", err)
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
		// If dest contains spaces, usually you'd use JSON format, but for simplicity
		// we treat the first space-separated string as src, and the rest as dest.
		copyParts := strings.SplitN(arg, " ", 2)
		if len(copyParts) < 2 {
			return fmt.Errorf("COPY requires source and destination arguments")
		}
		return p.executor.EvalCOPY(copyParts[0], strings.TrimSpace(copyParts[1]))
		
	case "RUN":
		if arg == "" {
			return fmt.Errorf("RUN requires a command argument")
		}
		return p.executor.EvalRUN(arg)
		
	default:
		return fmt.Errorf("Unknown instruction '%s'", instruction)
	}
}
