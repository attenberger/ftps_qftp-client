package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/attenberger/ftps_qftp-client"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	var (
		port = flag.Int("port", 2121, "Port")
		host = flag.String("host", "localhost", "Port")
		cert = flag.String("cert", "", "Path to server certificate for TLS")
	)
	flag.Parse()
	messageAboutMissingParameters := ""
	if *cert == "" {
		messageAboutMissingParameters = messageAboutMissingParameters + "Please set a certificatefile for the server with -cert\n"
	}
	if messageAboutMissingParameters != "" {
		log.Fatalf(messageAboutMissingParameters)
	}

	command := generateFunctionsMap()
	consoleReader := bufio.NewReader(os.Stdin)

	connection, err := client_ftp.DialTimeout(*host+":"+strconv.Itoa(*port), time.Second*30, *cert)
	if err != nil {
		fmt.Println("Error opening connection to server: " + err.Error())
		return
	}
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	for {
		fmt.Print("> ")
		line, incompleteline, err := consoleReader.ReadLine()
		if err != nil {
			fmt.Println("Error while reading command: " + err.Error())
			continue
		}
		if incompleteline {
			fmt.Println("Command was to long.")
			continue
		}
		commandParts := strings.Split(string(line), " ")
		function, available := command[strings.ToUpper(commandParts[0])]
		if available {
			function(connection, commandParts[1:]...)
		} else {
			fmt.Println("Command at this client not available.")
		}
		if commandParts[0] == "QUIT" {
			return
		}
	}
}

func generateFunctionsMap() map[string]func(connection *client_ftp.ServerConn, parameters ...string) {

	var functions = make(map[string]func(connection *client_ftp.ServerConn, parameters ...string))

	functions["AUTH"] = func(connection *client_ftp.ServerConn, parameters ...string) {
		if len(parameters) != 1 {
			fmt.Println("Please use AUTH-command in the following pattern \"AUTH Method\".")
		} else if strings.ToUpper(parameters[0]) != "TLS" {
			fmt.Println("Just TLS authentication method is supported.")
		} else {
			err := connection.AuthTLS()
			if err != nil {
				fmt.Println(err.Error())
			}
		}
	}

	functions["LOGIN"] = func(connection *client_ftp.ServerConn, parameters ...string) {
		if len(parameters) != 2 {
			fmt.Println("Please use LOGIN-command in the following pattern \"LOGIN Username Password\".")
		} else {
			err := connection.Login(parameters[0], parameters[1])
			if err != nil {
				fmt.Println(err.Error())
			}
		}
	}

	functions["QUIT"] = func(connection *client_ftp.ServerConn, parameters ...string) {
		err := connection.Quit()
		if err != nil {
			fmt.Println(err.Error())
		}
	}

	/*
		connection.ChangeDir()
				connection.ChangeDirToParent()
				connection.CurrentDir()
				connection.Delete()
				connection.Feat()
				connection.Features()
				connection.List()
				connection.Logout()
				connection.MakeDir()
				connection.NameList()
				connection.NoOp()
				connection.RemoveDir()
				connection.Rename()
				connection.Retr()
				connection.RetrFrom()
				connection.Stor()
				connection.StorFrom()
	*/

	return functions
}
