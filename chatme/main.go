// chatme runs a subcommand and sends its exit status to Google Chat.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// minTime is the minimum subcommand execution time for sending a chat
// message. If the command completes in less than minTime, we don't
// send a chat message.
const minTime = 10 * time.Second

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: chatme command...\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	command := flag.Args()

	// Load configuration.
	config := loadConfig()

	// Run command.
	startTime := time.Now()
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		// Failures like "executable not found".
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	// Ignore signals that should go to the subcommand.
	signal.Ignore(os.Interrupt, syscall.SIGQUIT)

	// Wait for command completion.
	err = cmd.Wait()
	res, ok := err.(*exec.ExitError)
	if err != nil && !ok {
		// I/O errors.
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	// Build chat message.
	var msg string
	cmdline := shQuote(command)
	if limit := 100; len(cmdline) > limit {
		cmdline = cmdline[:limit-3] + "..."
	}
	if res == nil {
		msg = fmt.Sprintf("Success: `%s`", cmdline)
	} else {
		msg = fmt.Sprintf("%s: `%s`", res.ProcessState, cmdline)
	}

	// Send chat message.
	if time.Since(startTime) >= minTime {
		sendChat(config.webhookUrl, msg)
	}

	// Exit with cmd's status.
	if res == nil {
		os.Exit(0)
	} else if res.Exited() {
		os.Exit(res.ExitCode())
	} else {
		fmt.Fprintf(os.Stderr, "%s\n", res.ProcessState)
		os.Exit(1)
	}
}

type config struct {
	webhookUrl string
}

func loadConfig() *config {
	dir, err := os.UserConfigDir()
	if err != nil {
		log.Fatalf("opening configuration: %v", err)
	}

	webhook, err := os.ReadFile(filepath.Join(dir, "chatme", "webhook"))
	if os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, `Webhook URL not configured. In the Chat room menu, select "Manage webhooks",
add a webhook, and paste the generated webhook URL in
%s/chatme/webhook .
`, dir)
		os.Exit(1)
	} else if err != nil {
		log.Fatalf("opening configuration: %v", err)
	}
	webhook = bytes.TrimSpace(webhook)

	return &config{string(webhook)}
}

var noQuote = regexp.MustCompile(`^[-a-zA-Z0-9/:_,.]+$`)
var doubleQuote = regexp.MustCompile(`^[-a-zA-Z0-9/:_,. *+=[]{}|;'<>?]*$`)

func shQuote(words []string) string {
	var out strings.Builder
	for i, word := range words {
		if i > 0 {
			out.WriteByte(' ')
		}
		// Be pretty strict about what doesn't require
		// quoting.
		if noQuote.MatchString(word) {
			out.WriteString(word)
		} else if doubleQuote.MatchString(word) {
			// We can double quote without any escaping.
			out.WriteByte('"')
			out.WriteString(word)
			out.WriteByte('"')
		} else {
			out.WriteByte('\'')
			out.WriteString(strings.Replace(word, `'`, `'"'"'`, -1))
			out.WriteByte('\'')
		}
	}
	return out.String()
}

type chatMessage struct {
	Text string `json:"text"`
}

// For the format of msg, see
// https://developers.google.com/hangouts/chat/reference/message-formats/basic
// (there is also a fancy card format that we could try out).
func sendChat(webhookUrl string, msg string) {
	body, err := json.Marshal(&chatMessage{Text: msg})
	if err != nil {
		log.Fatalf("error encoding chat message: %v", err)
	}

	// Chat webhooks doc: https://developers.google.com/hangouts/chat/how-tos/webhooks
	resp, err := http.Post(webhookUrl, "application/json; charset=UTF-8", bytes.NewBuffer(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error sending chat: %v", err)
		return
	}

	if resp.StatusCode != 200 {
		err, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "error sending chat: %s\n%s\n", resp.Status, err)
	}

	resp.Body.Close()
}
