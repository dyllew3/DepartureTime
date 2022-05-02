package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/cockroach-go/v2/crdb/crdbpgx"
	"github.com/jackc/pgx/v4"
	"golang.org/x/net/html"
)

const AirportPage string = "https://www.dublinairport.com/flight-information/live-departures"
const NumTerminals int = 2
const TerminalOne string = "T1"
const TerminalTwo string = "T2"

var Terminals []string = []string{TerminalOne, TerminalTwo}

type TerminalSecurityRecord struct {
	Terminal  string    `json:"terminal"`
	TimeStamp time.Time `json:"timestamp"`
	WaitLen   int       `json:"waitlen"`
}

// Renders node into text, takes in html node and returns a string
func renderNode(n *html.Node) string {
	var buf bytes.Buffer
	w := io.Writer(&buf)
	html.Render(w, n)
	return buf.String()
}

// From html gets the div element which contains
func getSecurityTimesNode(n *html.Node) *html.Node {
	if n == nil {
		return nil
	}
	if n.Type == html.ElementNode && n.Data == "div" {
		for _, a := range n.Attr {
			if a.Val == "sec-times" {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		var node *html.Node = getSecurityTimesNode(c)
		if node != nil {
			return node
		}
	}
	return nil
}

// From the string "= x mins" extracts the "x" as its the minute value
func getMinuteValue(text string) string {
	removedMin := strings.ReplaceAll(text, "min", "")
	removedEquals := strings.ReplaceAll(removedMin, "=", "")
	removedSpaces := strings.ReplaceAll(removedEquals, " ", "")
	return removedSpaces
}

// Get the raw text in a html node
func getRawText(n *html.Node) []string {
	var rawText []string = make([]string, 0)

	// Get text stating how many minutes security text
	for c := n.NextSibling; c != nil; c = c.NextSibling {
		result := getStrongTagNode(c)
		if result != nil {
			var textNode *html.Node = result.FirstChild
			// Get text from node but remove trailing spaces
			if textNode != nil {
				stringVal := strings.TrimSpace(renderNode(textNode))
				rawText = append(rawText, stringVal)
			}
		}
	}
	return rawText
}

// Gets from tag from current html node, either this node or one of its children
// returns nil if it is unable to find strong tag
func getStrongTagNode(n *html.Node) *html.Node {
	if n == nil {
		return nil
	}

	// If node is html strong tag return it
	if n.Type == html.ElementNode && n.Data == "strong" {
		return n
	}

	// Check if child nodes have strong tag
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		var node *html.Node = getStrongTagNode(c)
		if node != nil {
			return node
		}
	}
	return nil
}

// Convert the raw text and the current time into an array of terminal records
func createTerminalRecords(rawText []string, timestamp time.Time) []TerminalSecurityRecord {
	var records []TerminalSecurityRecord = make([]TerminalSecurityRecord, 0)

	// Convert text to integers
	for i, val := range rawText {
		intVal, err := strconv.Atoi(getMinuteValue(val))
		if err != nil {
			log.Printf("encountered error when converting minute value %s for terminal %s \n", val, Terminals[i])
			return []TerminalSecurityRecord{}
		}
		newRecord := TerminalSecurityRecord{
			Terminal:  Terminals[i],
			TimeStamp: timestamp,
			WaitLen:   intVal,
		}
		records = append(records, newRecord)
	}
	return records
}

// This can be used to write data to a json file instead of a database
func writeDataToJsonFile(records []TerminalSecurityRecord) error {
	year, month, day := time.Now().Date()
	var filename string = fmt.Sprintf("./data/%d-%s-%d.json", year, month.String(), day)

	var dataList []TerminalSecurityRecord = []TerminalSecurityRecord{}

	// create file if it doesn't exist
	if _, err := os.Stat(filename); err != nil {
		file, _ := os.Create(filename)
		file.WriteString("[]")
		err = file.Close()
		if err != nil {
			return err
		}
	}
	file, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	// Parse file data to json
	err = json.Unmarshal(file, &dataList)
	if err != nil {
		return err
	}
	dataList = append(dataList, records...)
	jsonBytes, err := json.Marshal(dataList)
	os.WriteFile(filename, jsonBytes, 0644)

	return nil
}

// Inserts rows into database requires context, transaction and the data to add should return nil on success
func insertRows(ctx context.Context, tx pgx.Tx, data []TerminalSecurityRecord) error {
	// Insert rows into the "terminalrecords" table.
	log.Println("Creating new rows...")
	valueStr := ""
	for i, val := range data {
		var timeStmp time.Time = val.TimeStamp
		valueStr += fmt.Sprintf("('%s', %v, '%s')", val.Terminal, val.WaitLen, timeStmp.Format(time.RFC3339))
		if i != (len(data) - 1) {
			valueStr += ", "
		}
	}
	if _, err := tx.Exec(ctx,
		"INSERT INTO terminalrecords (terminal, wait_len, timestamp) VALUES "+valueStr); err != nil {
		return err
	}
	return nil
}

// Prints the current terminalrecords rows in the database
func printTerminalData(conn *pgx.Conn) error {
	rows, err := conn.Query(context.Background(), "SELECT * FROM terminalrecords")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var terminal string
		var waitlen int
		var timeStmp time.Time
		if err := rows.Scan(&terminal, &waitlen, &timeStmp); err != nil {
			log.Fatal(err)
		}
		log.Printf("%s %d %s\n", terminal, waitlen, timeStmp.String())
	}
	return nil
}

func main() {
	conn, err := pgx.Connect(context.Background(), os.Getenv("DB_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close(context.Background())

	for {

		// Restablish connection if lost
		if conn.IsClosed() || conn.Ping(context.Background()) != nil {
			conn, err = pgx.Connect(context.Background(), os.Getenv("DB_URL"))
			if err != nil {
				log.Fatal(err)
			}
			defer conn.Close(context.Background())
		}
		log.Println("Getting and submitting airport departure data.")
		departuresPage, err := http.Get(AirportPage)
		var timestamp time.Time = time.Now()
		if err != nil {
			log.Fatalln("Unable to load page " + AirportPage)
		} else {
			doc, err := html.Parse(departuresPage.Body)
			if err != nil {
				log.Fatalln(err)
			}

			// Get div node containing security times for each terminal
			n := getSecurityTimesNode(doc)

			if n == nil {
				log.Println("Unable to find tag with attribute 'sec-times'")
				return
			}
			n = n.FirstChild
			if n == nil {
				log.Println("No children of tag with attribute 'sec-times'")
			}

			var rawText []string = getRawText(n)
			departuresPage.Body.Close()

			if len(rawText) != 2 {
				log.Fatalln("Got " + fmt.Sprint(len(rawText)) + " should only be 2 terminal values")
			}

			var records []TerminalSecurityRecord = createTerminalRecords(rawText, timestamp)

			// Only display current data in db if specified
			if strings.ToLower(os.Getenv("SHOW_ROWS")) == "true" {
				// print current data
				err = printTerminalData(conn)

				if err != nil {
					log.Fatalln(err)
				}
			}

			// Actually add rows
			if strings.ToLower(os.Getenv("ADD_ROWS")) == "true" {
				// Add new data
				err = crdbpgx.ExecuteTx(context.Background(), conn, pgx.TxOptions{}, func(tx pgx.Tx) error {
					return insertRows(context.Background(), tx, records)
				})
				if err != nil {
					log.Fatalln(err)
				} else {
					log.Println("Successfully added rows")
				}
			} else {
				log.Println("ADD_ROWS not set to true so not adding rows")
			}
		}
		log.Println("Finished submitting data, loop will begin again in 10 minutes.")
		// Sleep for ten minutes
		time.Sleep(time.Minute * 10)
	}
}
