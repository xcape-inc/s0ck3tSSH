package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/RouterScript/ProxyClient"
)

func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func PublicKeyFile(file string) ssh.AuthMethod {
	buffer, err := ioutil.ReadFile(file)
	checkErr(err)

	key, err := ssh.ParsePrivateKey(buffer)
	checkErr(err)
	return ssh.PublicKeys(key)
}

func KIChallenge() ssh.KeyboardInteractiveChallenge {
	return func(user, instruction string, questions []string, echos []bool) ([]string, error) {
		answers := make([]string, len(questions))
		for questionIndex, question := range questions {
			if strings.Contains(question, "Password") {
				fmt.Println(question)
				bytePassword, err := terminal.ReadPassword(int(syscall.Stdin))
				checkErr(err)
				answers[questionIndex] = string(bytePassword)
			}
		}
		return answers, nil
	}
}

func main() {
	// check args
	if len(os.Args) < 3 {
		log.Fatal("USAGE: ", os.Args[0], " <proxy host> <ssh host> [key]")
	}

	// ask for username
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Username:")
	username, err := reader.ReadString('\n')
	checkErr(err)

	// create config
	sshConfig := &ssh.ClientConfig{
		User:            strings.TrimSpace(username),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	if len(os.Args) > 3 { // key auth
		sshConfig.Auth = append(sshConfig.Auth, PublicKeyFile(os.Args[3]))
	} else { // password auth
		sshConfig.Auth = append(sshConfig.Auth, ssh.KeyboardInteractive(KIChallenge()))
	}

	// connect to proxy
	proxyURL, err := url.Parse(os.Args[1])
	checkErr(err)
	proxyObj, err := proxyclient.NewClient(proxyURL)
	checkErr(err)
	sshSocket, err := proxyObj.Dial("tcp", os.Args[2])
	checkErr(err)

	// connect to ssh
	clientConnection, clientChans, clientReqs, err := ssh.NewClientConn(sshSocket, os.Args[2], sshConfig)
	checkErr(err)
	connection := ssh.NewClient(clientConnection, clientChans, clientReqs)

	// create session
	sshSess, err := connection.NewSession()
	checkErr(err)

	// create psudo term
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.IGNCR:         1,     // Ignore CR on input.
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
	if err := sshSess.RequestPty("dumb", 80, 40, modes); err != nil {
		sshSess.Close()
		log.Fatal("request for pseudo terminal failed: " + err.Error())
	}

	// Start remote shell
	stdin, err := sshSess.StdinPipe()
	checkErr(err)
	go io.Copy(stdin, os.Stdin)
	stdout, err := sshSess.StdoutPipe()
	checkErr(err)
	go io.Copy(os.Stdout, stdout)
	stderr, err := sshSess.StderrPipe()
	checkErr(err)
	go io.Copy(os.Stderr, stderr)
	if err := sshSess.Shell(); err != nil {
		log.Fatalf("failed to start shell: %s", err)
	}

	// wait for kill switch
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	for sig := range c {
		log.Fatal(sig.String())
	}

	fmt.Println("Press Crtl-C to exit")
}
