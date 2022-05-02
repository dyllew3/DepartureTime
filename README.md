# DepartureTimes

The purpose of this project is to take the information on https://www.dublinairport.com/flight-information/live-departures
and record the amount of time they estimate it to get through security for terminals 1 and 2. This runs every 10 minutes and stores
the data in an sql database allowing for potential long term observations of the data. There is also functionality to write the data
to a json file. Writing to a JSON file with the current date as the name and if it already exists just appending the data.

## Database

Currently I am using cockroach DB to store the data with the table structure outlined in TerminalRecords.sql

## Setup

Golang 1.18 is all that should be required for the code to run.
But you will also need a database of some kind to store the data
along with some environment varaibles:

+ DB_URL the url of the database to add the data to
+ ADD_ROWS whether to add rows or not, set to "true" to add rows
+ SHOW_ROWS whether to show current rows in db, set to "true" to show rows

## Parsing the html

index.html contains the html page of dublin airport security times and example.html contains what the div we actually care about is formatted like.
