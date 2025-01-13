package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

const smtpPort = ":2525"

var DB *sql.DB

func main() {
	logFile := setupLogging()
	defer logFile.Close()

	db, err := setupDatabase()
	if err != nil {
		log.Fatalf("Failed to set up database: %v", err)
	}
	DB = db
	defer db.Close()

	listener, err := net.Listen("tcp", smtpPort)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	defer listener.Close()
	log.Printf("SMTP server is running on port %s...", smtpPort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	writer := bufio.NewWriter(conn)
	reader := bufio.NewReader(conn)

	// Send initial SMTP greeting
	writer.WriteString("220 Welcome to Go SMTP Server\r\n")
	writer.Flush()

	var sender, recipient, data string

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Connection error: %v", err)
			return
		}

		line = strings.TrimSpace(line)
		log.Printf("Received: %s", line)

		switch {
		case strings.HasPrefix(strings.ToUpper(line), "HELO"):
			writer.WriteString("250 Hello\r\n")
		case strings.HasPrefix(strings.ToUpper(line), "MAIL FROM:"):
			sender = strings.TrimSpace(strings.TrimPrefix(line, "MAIL FROM:"))
			writer.WriteString("250 OK\r\n")
		case strings.HasPrefix(strings.ToUpper(line), "RCPT TO:"):
			recipient = strings.TrimSpace(strings.TrimPrefix(line, "RCPT TO:"))
			writer.WriteString("250 OK\r\n")
		case strings.HasPrefix(strings.ToUpper(line), "DATA"):
			writer.WriteString("354 End data with <CR><LF>.<CR><LF>\r\n")
			writer.Flush()

			data = ""
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					log.Printf("Error reading data: %v", err)
					return
				}
				if strings.TrimSpace(dataLine) == "." {
					break
				}
				data += dataLine
			}


			log.Printf("Email received:\nFrom: %s\nTo: %s\nData: %s", sender, recipient, data)
			if err := saveEmail(sender, recipient, data); err != nil {
				log.Printf("Failed to save email: %v", err)
				writer.WriteString("451 Requested action aborted: local error in processing\r\n")
			} else {
				writer.WriteString("250 OK\r\n")
			}

			err = saveEmailToDB(DB, sender, recipient, data)
			if err != nil {
				log.Printf("Failed to save email to database: %v", err)
				writer.WriteString("500 Failed to save email\r\n")
			} else {
				writer.WriteString("250 OK\r\n")
			}
		case strings.HasPrefix(strings.ToUpper(line), "QUIT"):
			writer.WriteString("221 Bye\r\n")
			writer.Flush()
			return

		default:
			writer.WriteString("500 Command not recognized\r\n")
		}

		writer.Flush()
	}
}

func setupLogging() *os.File {
	logFile, err := os.OpenFile("smtp_server.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	log.SetOutput(logFile)
	return logFile
}

func saveEmail(sender, recipient, data string) error {
	emailFile, err := os.OpenFile("emails.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open email file: %v", err)
	}
	defer emailFile.Close()

	emailRecord := fmt.Sprintf("From: %s\nTo: %s\nData:\n%s\n---\n", sender, recipient, data)
	_, err = emailFile.WriteString(emailRecord)
	return err
}

func setupDatabase() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "emails.db")
	if err != nil {
		return nil, err
	}

	createTableQuery := `
    CREATE TABLE IF NOT EXISTS emails (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        sender TEXT,
        recipient TEXT,
        data TEXT,
        timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
    );`
	_, err = db.Exec(createTableQuery)
	return db, err
}

func saveEmailToDB(db *sql.DB, sender, recipient, data string) error {
	_, err := db.Exec("INSERT INTO emails (sender, recipient, data) VALUES (?, ?, ?)", sender, recipient, data)
	return err
}
