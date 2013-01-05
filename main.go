package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jfox85/goconfig/config"
	_ "github.com/jfox85/mymysql/godrv"
)

const (
	DEFAULT_CONFIG_FILE = "config.txt"
)

var (
	// Errors
	ErrSpecifyUrl = errors.New("Please specify a url parameter")

	con     *sql.DB // DB connection
	baseUrl string  // Base url of short urls
)

func main() {
	var configFileName string

	// Grab the config file path from the command line flags
	flag.StringVar(&configFileName, "config", DEFAULT_CONFIG_FILE, "config file path")
	flag.Parse()

	// Load the config file
	configs, configErr := config.ReadDefault(configFileName)
	handleFatalErr(configErr)

	// Grab the values we need from it
	var dbName, dbUser, dbPass, listenPort string
	var timeout int

	dbName, configErr = configs.String("DEFAULT", "db-name")
	handleFatalErr(configErr)

	dbUser, configErr = configs.String("DEFAULT", "db-user")
	handleFatalErr(configErr)

	dbPass, configErr = configs.String("DEFAULT", "db-pass")
	handleFatalErr(configErr)

	listenPort, configErr = configs.String("DEFAULT", "listen-port")
	handleFatalErr(configErr)

	timeout, configErr = configs.Int("DEFAULT", "timeout")
	handleFatalErr(configErr)

	dbPass, configErr = configs.String("DEFAULT", "db-pass")
	handleFatalErr(configErr)

	baseUrl, configErr = configs.String("DEFAULT", "base-url")
	handleFatalErr(configErr)

	baseUrl = strings.TrimRight(baseUrl, "/")

	// Open the database connection
	var err error
	con, err = sql.Open("mymysql", dbName+"/"+dbUser+"/"+dbPass)
	defer con.Close()
	handleFatalErr(err)

	// Add the URL hooks
	http.HandleFunc("/", urlHandler)
	http.HandleFunc("/add-url/", addUrlHandler)

	// Create the server
	s := &http.Server{
		Addr:           ":" + listenPort,
		ReadTimeout:    time.Duration(timeout) * time.Second,
		WriteTimeout:   time.Duration(timeout) * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	// Start listening...
	log.Fatal(s.ListenAndServe())
}

// Log the error and die
func handleFatalErr(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

// urlHandler deals with an existing short url and forwards it to the original url
func urlHandler(w http.ResponseWriter, r *http.Request) {
	lookupUrl := strings.Trim(r.URL.Path, "/ \n")
	expandedUrl := getUrlFromShortcode(lookupUrl)

	var expandedLogUrl string
	if expandedUrl == "" {
		expandedLogUrl = "NULL"
	} else {
		expandedLogUrl = expandedUrl
	}
	log.Println("URL Lookup Request: /" + lookupUrl + " => " + expandedLogUrl + " " + r.RemoteAddr + " " + r.UserAgent() + " " + r.Referer())

	if expandedUrl == "" {
		w.Header().Add("Content-Type", "text/html")
		w.WriteHeader(404)
		fmt.Fprintln(w, "That url doesn't seem to exist.  Maybe try checking out my blog instead? <a href='http://jonefox.com/blog/'>http://jonefox.com/blog/</a>")
		return
	}

	w.Header().Add("Location", expandedUrl)
	w.WriteHeader(301)
}

// Get the url that a short code should expand to
func getUrlFromShortcode(shortcode string) string {
	// Check if the tweet is already in the DB
	row := con.QueryRow("SELECT `url` FROM `urls` WHERE `short_code`=? LIMIT 1", shortcode)

	var url string
	rowErr := row.Scan(&url)
	if rowErr == sql.ErrNoRows {
		return ""
	}

	return url
}

// addUrlHandler handles adding new urls to the DB and returns the short url version
func addUrlHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Add URL Request: " + r.URL.Query().Get("url") + " " + r.RemoteAddr + " " + r.UserAgent() + " " + r.Referer())
	urlToAdd := r.URL.Query().Get("url")
	if urlToAdd == "" {
		outputWebErr(w, ErrSpecifyUrl)
		return
	}

	// Check if the url already exists - if it does return the short code we already have, if it doesn't add it
	row := con.QueryRow("SELECT `short_code` FROM `urls` WHERE `url`=? LIMIT 1", urlToAdd)

	var shortcode string
	rowErr := row.Scan(&shortcode)
	if rowErr == sql.ErrNoRows {
		// Insurt the url
		result, err := con.Exec("INSERT INTO `urls` (url) VALUES (?)", urlToAdd)
		if err != nil {
			outputWebErr(w, err)
			return
		}

		// Grab the insertId
		id, idErr := result.LastInsertId()
		if idErr != nil {
			outputWebErr(w, idErr)
			return
		}

		// Create a shortcode and update the url to use it
		shortcode = strconv.FormatInt(id, 36)
		_, updateErr := con.Exec("UPDATE `urls` SET `short_code` = ? WHERE `id` = ? LIMIT 1", shortcode, id)
		if updateErr != nil {
			outputWebErr(w, updateErr)
			return
		}
	}

	// Output the short url
	w.WriteHeader(200)
	fmt.Fprintln(w, baseUrl+"/"+shortcode)
}

// Output a 500 error to the HTTP client with a custom message
func outputWebErr(w http.ResponseWriter, err error) {
	w.WriteHeader(500)
	fmt.Fprintln(w, err.Error())
}
