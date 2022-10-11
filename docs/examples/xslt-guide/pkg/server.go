package main

import (
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/foomo/soap"
	"github.com/lithammer/fuzzysearch/fuzzy"
)

// Query a simple request
type Query struct {
	XMLName   xml.Name `xml:"Query"`
	CityQuery string
}

type Match struct {
	City       string
	Country    string
	SubCountry string
	GeoNameId  string
}

// FoundResponse a simple response
type FoundResponse struct {
	Match []Match
}

func GetCities() ([]string, []string, []string, []string, error) {
	var cities, countries, subcountry, geonameids []string
	csvPath := "/usr/local/bin/world_cities.csv"
	if envPath := os.Getenv("CITY_CSV_PATH"); envPath != "" {
		csvPath = envPath
	}
	csvfile, err := os.Open(csvPath)
	if err != nil {
		log.Println("Couldn't open the csv file", err)
		return nil, nil, nil, nil, err

	}
	r := csv.NewReader(csvfile)
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Println(err)
			return nil, nil, nil, nil, err
		}
		cities = append(cities, strings.ToLower(record[0]))
		countries = append(countries, record[1])
		subcountry = append(subcountry, record[2])
		geonameids = append(geonameids, record[3])
	}
	return cities, countries, subcountry, geonameids, nil
}

// Fuzzy finds city in list of city names
func FindCity(cities []string, query string) (bool, []int) {
	ranks := fuzzy.RankFind(query, cities)
	if len(ranks) < 1 {
		return false, nil
	}
	sort.Sort(ranks)
	var indicies []int
	for _, rank := range ranks {
		indicies = append(indicies, rank.OriginalIndex)
	}
	return true, indicies
}

// RunServer run a little demo server
func RunServer() {
	cities, countries, subcountry, geonameids, err := GetCities()
	if err != nil {
		fmt.Println("exiting with error", err)
		return
	}
	soapServer := soap.NewServer()
	soapServer.RegisterHandler(
		"/",
		// SOAPAction
		"findCity",
		// tagname of soap body content
		"Query",
		// RequestFactoryFunc - give the server sth. to unmarshal the request into
		func() interface{} {
			return &Query{}
		},
		// OperationHandlerFunc - do something
		func(request interface{}, w http.ResponseWriter, httpRequest *http.Request) (response interface{}, err error) {
			req := request.(*Query)
			found, idxs := FindCity(cities, strings.ToLower(req.CityQuery))
			if found {
				var matches []Match
				for _, idx := range idxs {
					matches = append(matches, Match{
						City:       cities[idx],
						Country:    countries[idx],
						SubCountry: subcountry[idx],
						GeoNameId:  geonameids[idx],
					})
				}
				response = &FoundResponse{
					Match: matches,
				}
			} else {
				err = fmt.Errorf("unable to find query %s in cities", req.CityQuery)
			}

			return
		},
	)
	err = soapServer.ListenAndServe(":8080")
	fmt.Println("exiting with error", err)
}

func main() {
	// see what is going on
	soap.Verbose = false
	RunServer()
}
