package main

import (
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"github.com/foomo/soap"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
)

// Query a simple request
type Query struct {
	XMLName   xml.Name `xml:"Query"`
	CityQuery string
}

// FoundResponse a simple response
type FoundResponse struct {
	City       string
	Country    string
	SubCountry string
	GeoNameId  string
}

func GetCities() ([]string, []string, []string, []string) {
	var cities, countries, subcountry, geonameids []string
	csvfile, err := os.Open("/Users/sai/go/src/github.com/solo-io/gloo/docs/examples/xslt-guide/pkg/world_cities.csv")
	if err != nil {
		log.Fatalln("Couldn't open the csv file", err)
	}
	r := csv.NewReader(csvfile)
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		cities = append(cities, strings.ToLower(record[0]))
		countries = append(countries, record[1])
		subcountry = append(subcountry, record[2])
		geonameids = append(geonameids, record[3])
	}
	return cities, countries, subcountry, geonameids
}

// Fuzzy finds city in list of city names
func FindCity(cities []string, query string) (bool, int) {
	ranks := fuzzy.RankFind(query, cities)
	if len(ranks) < 1 {
		return false, -1
	}
	sort.Sort(ranks)
	return true, ranks[0].OriginalIndex
}

// RunServer run a little demo server
func RunServer() {
	cities, countries, subcountry, geonameids := GetCities()
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
			found, idx := FindCity(cities, strings.ToLower(req.CityQuery))
			if found {
				response = &FoundResponse{
					City:       cities[idx],
					Country:    countries[idx],
					SubCountry: subcountry[idx],
					GeoNameId:  geonameids[idx],
				}
			} else {
				err = fmt.Errorf("unable to find query %s in cities", req.CityQuery)
			}

			return
		},
	)
	err := soapServer.ListenAndServe(":8080")
	fmt.Println("exiting with error", err)
}

func main() {
	// see what is going on
	soap.Verbose = true
	RunServer()
}
