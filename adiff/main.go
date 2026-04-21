package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [flags] file1 file2\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Colorized diff. Can be called like diff, or used as a filter.\n\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func parseSub(sub string) (*regexp.Regexp, string, error) {
	if len(sub) >= 3 && sub[0] == '/' && sub[len(sub)-1] == '/' {
		for i := 1; i < len(sub)-1; i++ {
			if sub[i] == '/' {
				slashes := 0
				for j := i - 1; j > 0 && sub[j] == '\\'; j-- {
					slashes++
				}
				if slashes%2 == 0 {
					reStr := sub[1:i]
					repl := sub[i+1 : len(sub)-1]
					repl = strings.ReplaceAll(repl, `\/`, `/`)
					re, err := regexp.Compile(reStr)
					return re, repl, err
				}
			}
		}
	}
	re, err := regexp.Compile(sub)
	return re, "…", err
}

func streamSub(filename string, re *regexp.Regexp, repl string, w *os.File) {
	defer w.Close()
	f, err := os.Open(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening %s: %v\n", filename, err)
		os.Exit(1)
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		hasNewline := false
		if len(line) > 0 {
			if line[len(line)-1] == '\n' {
				hasNewline = true
				line = line[:len(line)-1]
			}
			line = re.ReplaceAllString(line, repl)
			fmt.Fprint(w, line)
			if hasNewline {
				fmt.Fprint(w, "\n")
			}
		}
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", filename, err)
				os.Exit(1)
			}
			break
		}
	}
}

func main() {
	uFlag := flag.Int("u", 3, "Output NUM (default 3) lines of unified context")
	diffFlags := flag.String("diff-flags", "", "Space-separated list of arbitrary other arguments to pass to diff")
	subFlag := flag.String("sub", "", "Substitute regexp before diffing. Can be 'regexp' (implies '…' replacement) or '/before/replacement/'")

	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 2 {
		usage()
	}

	args := flag.Args()

	isStdoutTerminal := isTerminal(os.Stdout)

	file1, file2 := args[0], args[1]
	diffArgs := []string{fmt.Sprintf("-u%d", *uFlag), "-p"}
	if *diffFlags != "" {
		diffArgs = append(diffArgs, strings.Fields(*diffFlags)...)
	}
	if isStdoutTerminal {
		diffArgs = append(diffArgs, "--color=always")
	}

	var extraFiles []*os.File
	if *subFlag != "" {
		re, repl, err := parseSub(*subFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing sub regexp: %v\n", err)
			os.Exit(1)
		}
		var p1r, p1w, p2r, p2w *os.File
		p1r, p1w, err = os.Pipe()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating pipe: %v\n", err)
			os.Exit(1)
		}
		p2r, p2w, err = os.Pipe()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating pipe: %v\n", err)
			os.Exit(1)
		}
		go streamSub(file1, re, repl, p1w)
		go streamSub(file2, re, repl, p2w)

		extraFiles = []*os.File{p1r, p2r} // Child FDs 3 and 4.
		diffArgs = append(diffArgs, "--label", file1, "--label", file2, "/dev/fd/3", "/dev/fd/4")
	} else {
		diffArgs = append(diffArgs, file1, file2)
	}

	pin := exec.Command("diff", diffArgs...)
	pin.ExtraFiles = extraFiles
	stdout, err := pin.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating stdout pipe for diff: %v\n", err)
		os.Exit(1)
	}
	if err := pin.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting diff: %v\n", err)
		os.Exit(1)
	}

	var pout *exec.Cmd
	var outStream io.WriteCloser
	if isStdoutTerminal {
		// The -X is important in conjunction with -F on
		// terminals where 'te' clears the screen.
		pout = exec.Command("less", "-FXR")
		pout.Stdout = os.Stdout
		pout.Stderr = os.Stderr
		stdin, err := pout.StdinPipe()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating stdin pipe for less: %v\n", err)
			os.Exit(1)
		}
		outStream = stdin
		if err := pout.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting less: %v\n", err)
			os.Exit(1)
		}
	} else {
		outStream = nopCloser{os.Stdout}
	}

	_, errCopy := io.Copy(outStream, stdout)

	stdout.Close()

	status := 0
	if err := pin.Wait(); err != nil {
		status = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			status = exitErr.ExitCode()
			if statusSys, ok := exitErr.Sys().(syscall.WaitStatus); ok && statusSys.Signaled() {
				// Try to kill our own process with
				// the same signal.
				if pout != nil {
					pout.Process.Kill()
				}
				sig := statusSys.Signal()
				proc, _ := os.FindProcess(os.Getpid())
				proc.Signal(sig)
				os.Exit(128 + int(sig))
			}
		}
	}
	if errCopy != nil {
		status = 1
	}

	if pout != nil {
		outStream.Close()
		pout.Wait()
	}

	os.Exit(status)
}

type nopCloser struct {
	io.Writer
}

func (nopCloser) Close() error { return nil }
