// Commandline for the QUIC-FTP-Client to access an QUIC-FTP-Server
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
		port = flag.Int("port", 2120, "Port")
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
	subConnection, err := connection.GetNewSubConn()
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	username := ""
	password := ""

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
			fmt.Println("  MTRAN")
			for commandname := range commandMap {
				fmt.Println("  " + commandname)
			}
		} else if commandParts[0] == "MTRAN" {
			err = multipleTransfer(connection, subConnection, username, password, commandParts[1:]...)
			if err != nil {
				fmt.Println(err.Error())
			}
		} else {
			function, available := commandMap[commandParts[0]]
			if available {
				err = function(subConnection, commandParts[1:]...)
				if err != nil {
					fmt.Println(err.Error())
				} else if commandParts[0] == "LOGIN" {
					username = commandParts[1]
					password = commandParts[2]
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

// MultipleTransfer issues parallel FTP commands in parallel connections to store multiple files
// to the remote FTP server.
func multipleTransfer(connection *client_ftp.ServerConn, subConnection *client_ftp.ServerSubConn, username string, password string, parameters ...string) error {
	if len(parameters) < 4 || len(parameters)%3 != 1 {
		return errors.New("MTRAN needs at least four parameters. The first has to be the number of parallel subConnection, " +
			"the rest each a triple of transferdirection, local- and remotepath. Transferdirection is indicated by \"<\" " +
			" (retrieve from Server) and \">\" (store at server).")
	}
	parallelConnection, err := strconv.Atoi(parameters[0])
	if err != nil {
		return errors.New("Error converting number of parallel connections. " + err.Error())
	}
	tasks := make([]TransferTask, 0, (len(parameters)-1)/3)
	for i := 1; i < len(parameters); i = i + 3 {
		var direction TransferDirction
		switch parameters[i] {
		case "<":
			direction = Retrieve
		case ">":
			direction = Store
		default:
			return errors.New(parameters[i] + " is not a vaild transfer direction. \"<\" or \">\" expected.")
		}
		tasks = append(tasks, NewTransferTask(direction, parameters[i+1], parameters[i+2]))
	}
	currentdirctory, err := subConnection.CurrentDir()
	if err != nil {
		return err
	}

	// Not more connections than files to store or negative
	if len(tasks) < parallelConnection || parallelConnection < 0 {
		parallelConnection = len(tasks)
	}

	// Write all tasks to the channel including the finishing message
	taskChannel := make(chan TransferTask, len(tasks)+parallelConnection)
	returnChannel := make(chan error, len(tasks))
	for _, task := range tasks {
		task.finished = false
		taskChannel <- task
	}
	for i := 0; i < parallelConnection; i++ {
		taskChannel <- TransferTask{finished: true}
	}

	// Start goroutines for parallel connections and provide the channels for communication
	for i := 0; i < parallelConnection; i++ {
		subC, err := connection.GetNewSubConn()
		if err != nil {
			fmt.Println(err)
		} else {
			go parallelTransfer(subC, username, password, currentdirctory, taskChannel, returnChannel)
		}
	}
	// The main connection is also used for parallel transfer
	/*for {
		task := <-taskChannel
		if task.finished {
			break
		} else if task.direction == Store {
			returnChannel <- c.mainSubConn.parallelStorTask(task)
		} else if task.direction == Retrieve {
			returnChannel <- c.mainSubConn.parallelRetrTask(task)
		} else {
			returnChannel <- errors.New("Unknown direction for transfer.")
		}
	}*/

	errorMessage := ""
	// Wait for replais of the STORs in the goroutines
	for normalReplay, goRoutineResetReply := 0, 0; normalReplay < len(tasks) && goRoutineResetReply < parallelConnection; normalReplay++ {
		replay := <-returnChannel
		if replay != nil {
			errorMessage = errorMessage + "\n" + replay.Error()
			if strings.HasPrefix("Go routine reset.", replay.Error()) {
				goRoutineResetReply++
			}
		}
	}
	if errorMessage == "" {
		return nil
	} else {
		return errors.New(errorMessage)
	}
}

// Generates a map of functions for all supported commands of the userinterface.
// The commands are not necessarily FTP-Commands.
func generateFunctionsMap() map[string]func(subConnection *client_ftp.ServerSubConn, parameters ...string) error {

	var functions = make(map[string]func(subConnection *client_ftp.ServerSubConn, parameters ...string) error)

	functions["CDUP"] = func(subConnection *client_ftp.ServerSubConn, parameters ...string) error {
		if len(parameters) != 0 {
			return errors.New("CDUP accepts no parameter.")
		}
		return subConnection.ChangeDirToParent()
	}

	functions["CLD"] = func(subConnection *client_ftp.ServerSubConn, parameters ...string) error {
		if len(parameters) != 1 {
			return errors.New("CLD needs one parameter")
		}
		return os.Chdir(parameters[0])
	}

	functions["CWD"] = func(subConnection *client_ftp.ServerSubConn, parameters ...string) error {
		if len(parameters) < 1 {
			return errors.New("CWD needs one parameter.")
		}
		return subConnection.ChangeDir(parameters[0])
	}

	functions["DELE"] = func(subConnection *client_ftp.ServerSubConn, parameters ...string) error {
		if len(parameters) < 1 {
			return errors.New("DELE needs one parameter.")
		}
		return subConnection.Delete(parameters[0])
	}

	functions["FEAT"] = func(subConnection *client_ftp.ServerSubConn, parameters ...string) error {
		if len(parameters) != 0 {
			return errors.New("FEAT accepts no parameter.")
		}
		for _, feature := range subConnection.Features() {
			fmt.Println("  " + feature)
		}
		return nil
	}

	functions["LIST"] = func(subConnection *client_ftp.ServerSubConn, parameters ...string) error {
		var entrys []*client_ftp.Entry
		var err error
		switch len(parameters) {
		case 0:
			entrys, err = subConnection.List(".")
		case 1:
			entrys, err = subConnection.List(parameters[0])
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

	functions["LOGIN"] = func(subConnection *client_ftp.ServerSubConn, parameters ...string) error {
		if len(parameters) != 2 {
			return errors.New("Please use LOGIN-command in the following pattern \"LOGIN Username Password\".")
		}
		return subConnection.Login(parameters[0], parameters[1])
	}

	functions["LOGOUT"] = func(subConnection *client_ftp.ServerSubConn, parameters ...string) error {
		if len(parameters) != 0 {
			return errors.New("LOGOUT accepts no parameter.")
		}
		return subConnection.Logout()
	}

	functions["MKD"] = func(subConnection *client_ftp.ServerSubConn, parameters ...string) error {
		if len(parameters) < 1 {
			return errors.New("MKD needs one parameter.")
		}
		return subConnection.MakeDir(parameters[0])
	}

	functions["NLST"] = func(subConnection *client_ftp.ServerSubConn, parameters ...string) error {
		var entrys []string
		var err error
		switch len(parameters) {
		case 0:
			entrys, err = subConnection.NameList(".")
		case 1:
			entrys, err = subConnection.NameList(parameters[0])
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

	functions["NOOP"] = func(subConnection *client_ftp.ServerSubConn, parameters ...string) error {
		if len(parameters) != 0 {
			return errors.New("NOOP accepts no parameter.")
		}
		return subConnection.NoOp()
	}

	functions["QUIT"] = func(subConnection *client_ftp.ServerSubConn, parameters ...string) error {
		if len(parameters) != 0 {
			return errors.New("QUIT accepts no parameter.")
		}
		return subConnection.Quit()
	}

	functions["PWD"] = func(subConnection *client_ftp.ServerSubConn, parameters ...string) error {
		if len(parameters) != 0 {
			return errors.New("PWD accepts no parameter.")
		}
		currentdir, err := subConnection.CurrentDir()
		if err != nil {
			return err
		}
		fmt.Println("  " + currentdir)
		return nil
	}

	functions["RENAME"] = func(subConnection *client_ftp.ServerSubConn, parameters ...string) error {
		if len(parameters) != 2 {
			return errors.New("RENAME needs two parameters. Rename of files with whitespaces is in this version not possible.")
		}
		return subConnection.Rename(parameters[0], parameters[1])
	}

	functions["RETR"] = func(subConnection *client_ftp.ServerSubConn, parameters ...string) error {
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

		reader, err := subConnection.Retr(remotepath)
		if err != nil {
			return err
		}
		_, err = io.Copy(file, reader)
		if err != nil {
			errortext := "Error while writing file to local file. " + err.Error()
			err = reader.Close()
			if err != nil {
				errortext = errortext + " Error while closing reader from server. " + err.Error()
			}
			return errors.New(errortext)
		}
		err = reader.Close()
		if err != nil {
			return errors.New(" Error while closing reader from server. " + err.Error())
		}
		return nil
	}

	functions["RMD"] = func(subConnection *client_ftp.ServerSubConn, parameters ...string) error {
		if len(parameters) < 1 {
			return errors.New("RKD needs one parameter.")
		}
		return subConnection.RemoveDir(parameters[0])
	}

	functions["STOR"] = func(subConnection *client_ftp.ServerSubConn, parameters ...string) error {
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

		err = subConnection.Stor(remotepath, file)
		if err != nil {
			return errors.New("Error while writing file to server. " + err.Error())
		}
		return nil
	}

	return functions
}

type TransferDirction int8

const (
	Retrieve = TransferDirction(1)
	Store    = TransferDirction(2)
)

// Task to inform a go routine which transfer should be performed
type TransferTask struct {
	localpath  string
	remotepath string
	direction  TransferDirction
	finished   bool
}

// Creates a new TransferTask
func NewTransferTask(direction TransferDirction, localpath string, remotepath string) TransferTask {
	return TransferTask{localpath: localpath, remotepath: remotepath, direction: direction, finished: false}
}

// Runs a parallel transfer.
// In the taskChannel it gets the TransferTask to perform.
// In the returnChannel it returns occured error or nil for success
func parallelTransfer(subC *client_ftp.ServerSubConn, username string, password string, dirctory string, taskChannel chan TransferTask, returnChannel chan error) {

	defer subC.Quit()
	// Login in
	err := subC.Login(username, password)
	if err != nil {
		returnChannel <- errors.New("Go routine reset. " + err.Error())
		return
	}
	// Change to directory of the main connection
	err = subC.ChangeDir(dirctory)
	if err != nil {
		returnChannel <- errors.New("Go routine reset. " + err.Error())
		return
	}

	// run tasks
	for {
		task := <-taskChannel
		if task.finished {
			return
		} else if task.direction == Store {
			returnChannel <- parallelStorTask(task, subC)
		} else if task.direction == Retrieve {
			returnChannel <- parallelRetrTask(task, subC)
		} else {
			returnChannel <- errors.New("Unknown direction for transfer.")
		}
	}
}

// Stores a file at the server within a parallel transfer.
func parallelStorTask(task TransferTask, subC *client_ftp.ServerSubConn) error {
	file, err := os.Open(task.localpath)
	defer file.Close()
	if err != nil {
		return errors.New("Error while opening the local file " + task.localpath + ". " + err.Error())
	}

	err = subC.Stor(task.remotepath, file)
	if err != nil {
		return errors.New("Error while writing file " + task.localpath + " to server. " + err.Error())
	}
	return nil
}

// Receives a file at the server within a parallel transfer.
func parallelRetrTask(task TransferTask, subC *client_ftp.ServerSubConn) error {
	// Check if file already exists at client
	if _, err := os.Stat(task.localpath); os.IsExist(err) {
		return errors.New("File with this name already exists in local folder.")
	}

	// Create and open the file
	file, err := os.Create(task.localpath)
	if err != nil {
		return errors.New("Error while creating the local file. " + err.Error())
	}
	defer file.Close()

	// Retrieve the file and write it to the filesystem
	reader, err := subC.Retr(task.remotepath)
	if err != nil {
		return err
	}
	_, err = io.Copy(file, reader)
	if err != nil {
		errortext := "Error while writing file to local file. " + err.Error()
		err = reader.Close()
		if err != nil {
			errortext = errortext + " Error while closing reader from server. " + err.Error()
		}
		return errors.New(errortext)
	}

	// Finalize retrieve of the file
	err = reader.Close()
	if err != nil {
		return errors.New(" Error while closing reader from server. " + err.Error())
	}
	return nil
}
