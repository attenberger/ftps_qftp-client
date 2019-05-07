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

	consoleReader := bufio.NewReader(os.Stdin)

	connection, responseCode, responseMessage, err := client_ftp.DialTimeout(*host+":"+strconv.Itoa(*port), time.Second*30)
	if err != nil {
		fmt.Println("Error opening connection to server: " + err.Error())
		return
	}
	fmt.Println(strconv.Itoa(responseCode) + " " + responseMessage)

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
		switch strings.ToUpper(commandParts[0]) {
		case "QUIT":
			responseCode, responseMessage, err = connection.Quit()
			if err != nil {
				fmt.Println("Error while closing connection: " + err.Error())
			} else {
				fmt.Println(strconv.Itoa(responseCode) + " " + responseMessage)
				fmt.Println("Connection closed.")
				return
			}
		default:
			fmt.Println("Command at this client not available.")
		}
	}
}
