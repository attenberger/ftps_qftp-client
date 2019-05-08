// Commandline for the FTP-Client to access an FTP-Server over FTPS
// Arguments for starting the client are -cert (mandatory), -host and -port
// to specify the servers TLS-/X.509-certificate (filename), his hostname and
// controlport.

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"github.com/attenberger/ftps_qftp-client"
	"io"
	"log"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"
)

func main() {
	// Parse commandline flags
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

	// set working directory
	currentUser, err := user.Current()
	if err != nil {
		fmt.Println("Unable to read the current currentUser, to find out the local home directory.")
	}
	err = os.Chdir(currentUser.HomeDir)
	if err != nil {
		fmt.Println("Error changing working directory.")
	}

	// prepare necessary utils
	commandMap := generateFunctionsMap()
	consoleReader := bufio.NewReader(os.Stdin)

	// setup ftp connection
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
		// Read Command from Commandline
		fmt.Print("> ")
		line, incompleteline, err := consoleReader.ReadLine()
		if err != nil {
			fmt.Println("Error while reading commandMap: " + err.Error())
			continue
		}
		if incompleteline {
			fmt.Println("Command was to long.")
			continue
		}

		// Execute Command
		commandParts := strings.Split(string(line), " ")
		commandParts[0] = strings.ToUpper(commandParts[0])
		if commandParts[0] == "HELP" {
			if len(commandParts) != 1 {
				fmt.Println("Just without an argument implemented.")
				continue
			}
			fmt.Println("  Available commands:")
			fmt.Println("  HELP")
			fmt.Println("  CLD")
			for commandname := range commandMap {
				fmt.Println("  " + commandname)
			}
		} else {
			function, available := commandMap[commandParts[0]]
			if available {
				err = function(connection, commandParts[1:]...)
				if err != nil {
					fmt.Println(err.Error())
				}
			} else {
				fmt.Println("Command at this client not available.")
			}
			if commandParts[0] == "QUIT" {
				return
			}
		}
	}
}

// Generates a map of functions for all supported commands of the userinterface.
// The commands are not necessarily FTP-Commands.
func generateFunctionsMap() map[string]func(connection *client_ftp.ServerConn, parameters ...string) error {

	var functions = make(map[string]func(connection *client_ftp.ServerConn, parameters ...string) error)

	functions["AUTH"] = func(connection *client_ftp.ServerConn, parameters ...string) error {
		if len(parameters) != 1 {
			return errors.New("Please use AUTH-command in the following pattern \"AUTH Method\".")
		} else if strings.ToUpper(parameters[0]) != "TLS" {
			return errors.New("Just TLS authentication method is supported.")
		}
		return connection.AuthTLS()
	}

	functions["CDUP"] = func(connection *client_ftp.ServerConn, parameters ...string) error {
		if len(parameters) != 0 {
			return errors.New("CDUP accepts no parameter.")
		}
		return connection.ChangeDirToParent()
	}

	functions["CLD"] = func(connection *client_ftp.ServerConn, parameters ...string) error {
		if len(parameters) != 1 {
			return errors.New("CLD needs one parameter")
		}
		return os.Chdir(parameters[0])
	}

	functions["CWD"] = func(connection *client_ftp.ServerConn, parameters ...string) error {
		if len(parameters) < 1 {
			return errors.New("CWD needs one parameter.")
		}
		return connection.ChangeDir(parameters[0])
	}

	functions["DELE"] = func(connection *client_ftp.ServerConn, parameters ...string) error {
		if len(parameters) < 1 {
			return errors.New("DELE needs one parameter.")
		}
		return connection.Delete(parameters[0])
	}

	functions["FEAT"] = func(connection *client_ftp.ServerConn, parameters ...string) error {
		if len(parameters) != 0 {
			return errors.New("FEAT accepts no parameter.")
		}
		for _, feature := range connection.Features() {
			fmt.Println("  " + feature)
		}
		return nil
	}

	functions["LIST"] = func(connection *client_ftp.ServerConn, parameters ...string) error {
		var entrys []*client_ftp.Entry
		var err error
		switch len(parameters) {
		case 0:
			entrys, err = connection.List(".")
		case 1:
			entrys, err = connection.List(parameters[0])
		default:
			return errors.New("LIST needs one or no parameter.")
		}
		if err != nil {
			return err
		}
		for _, entry := range entrys {
			var typeChar string
			switch entry.Type {
			case client_ftp.EntryTypeFile:
				typeChar = "-"
			case client_ftp.EntryTypeFolder:
				typeChar = "d"
			case client_ftp.EntryTypeLink:
				typeChar = "l"
			default:
				typeChar = "?"
			}
			fmt.Printf("  %s %12d %20s %s\n", typeChar, entry.Size, entry.Time.String(), entry.Name)
		}
		return nil
	}

	functions["LOGIN"] = func(connection *client_ftp.ServerConn, parameters ...string) error {
		if len(parameters) != 2 {
			return errors.New("Please use LOGIN-command in the following pattern \"LOGIN Username Password\".")
		}
		return connection.Login(parameters[0], parameters[1])
	}

	functions["LOGOUT"] = func(connection *client_ftp.ServerConn, parameters ...string) error {
		if len(parameters) != 0 {
			return errors.New("LOGOUT accepts no parameter.")
		}
		return connection.Logout()
	}

	functions["MKD"] = func(connection *client_ftp.ServerConn, parameters ...string) error {
		if len(parameters) < 1 {
			return errors.New("MKD needs one parameter.")
		}
		return connection.MakeDir(parameters[0])
	}

	functions["NLST"] = func(connection *client_ftp.ServerConn, parameters ...string) error {
		var entrys []string
		var err error
		switch len(parameters) {
		case 0:
			entrys, err = connection.NameList(".")
		case 1:
			entrys, err = connection.NameList(parameters[0])
		default:
			return errors.New("LIST needs one or no parameter.")
		}
		if err != nil {
			return err
		}
		for _, entry := range entrys {
			fmt.Println("  " + entry)
		}
		return nil
	}

	functions["NOOP"] = func(connection *client_ftp.ServerConn, parameters ...string) error {
		if len(parameters) != 0 {
			return errors.New("NOOP accepts no parameter.")
		}
		return connection.NoOp()
	}

	functions["QUIT"] = func(connection *client_ftp.ServerConn, parameters ...string) error {
		if len(parameters) == 0 {
			return errors.New("QUIT accepts no parameter.")
		}
		return connection.Quit()
	}

	functions["PWD"] = func(connection *client_ftp.ServerConn, parameters ...string) error {
		if len(parameters) != 0 {
			return errors.New("PWD accepts no parameter.")
		}
		currentdir, err := connection.CurrentDir()
		if err != nil {
			return err
		}
		fmt.Println("  " + currentdir)
		return nil
	}

	functions["RENAME"] = func(connection *client_ftp.ServerConn, parameters ...string) error {
		if len(parameters) != 2 {
			return errors.New("RENAME needs two parameters. Rename of files with whitespaces is in this version not possible.")
		}
		return connection.Rename(parameters[0], parameters[1])
	}

	functions["RETR"] = func(connection *client_ftp.ServerConn, parameters ...string) error {
		if len(parameters) != 2 {
			return errors.New("RETR needs two parameter.")
		}
		localpath := parameters[0]
		remotepath := parameters[1]

		if _, err := os.Stat(localpath); os.IsExist(err) {
			return errors.New("File with this name already exists in local folder.")
		}
		file, err := os.Create(localpath)
		defer file.Close()
		if err != nil {
			return errors.New("Error while creating the local file. " + err.Error())
		}

		reader, err := connection.Retr(remotepath)
		if err != nil {
			return err
		}
		defer reader.Close()
		_, err = io.Copy(file, reader)
		if err != nil {
			return errors.New("Error while writing file to local file. " + err.Error())
		}
		if file.Close() != nil {
			return errors.New("Error while closing local file. " + err.Error())
		}
		return nil
	}

	functions["RMD"] = func(connection *client_ftp.ServerConn, parameters ...string) error {
		if len(parameters) < 1 {
			return errors.New("RKD needs one parameter.")
		}
		return connection.RemoveDir(parameters[0])
	}

	functions["STOR"] = func(connection *client_ftp.ServerConn, parameters ...string) error {
		if len(parameters) != 2 {
			return errors.New("STOR needs two parameter.")
		}
		localpath := parameters[0]
		remotepath := parameters[1]

		file, err := os.Open(localpath)
		defer file.Close()
		if err != nil {
			return errors.New("Error while opening the local file. " + err.Error())
		}

		err = connection.Stor(remotepath, file)
		if err != nil {
			return errors.New("Error while writing file to server. " + err.Error())
		}
		return nil
	}

	return functions
}
