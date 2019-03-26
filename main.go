package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	httpclient "github.com/ddliu/go-httpclient"
	influxdb "github.com/influxdata/influxdb1-client/v2"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load(".env")
	sleepinterval, _ := time.ParseDuration("15m")

	influx, err := influxdb.NewHTTPClient(influxdb.HTTPConfig{
		Addr:     fmt.Sprintf("http://%s:%s", os.Getenv("INFLUX_HOST"), os.Getenv("INFLUX_PORT")), //"http://192.168.1.3:8086",
		Username: os.Getenv("INFLUX_USER"),
		Password: os.Getenv("INFLUX_PASS"),
	})
	if err != nil {
		log.Fatalln(err)
		return
	}
	log.Print("Connected to InfluxDB")
	defer influx.Close()
	_, err = influx.Query(influxdb.NewQuery(fmt.Sprintf("CREATE DATABASE %s", os.Getenv("INFLUX_DB")), "", "s"))
	if err != nil {
		log.Fatalln(err)
		return
	}

	users := strings.Split(os.Getenv("MYAUSSIE_USER"), ",")
	passwords := strings.Split(os.Getenv("MYAUSSIE_PASS"), ",")

	if len(users) != len(passwords) {
		log.Fatal("Usernames and password lengths do not match at all.")
		return
	}

	for {
		for idx, user := range users {
			go parseForUser(user, passwords[idx], influx)
		}
		time.Sleep(sleepinterval)
	}
}

//NBNService -
type NBNService struct {
	ServiceID   int    `json:"service_id"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Plan        string `json:"plan"`
	Description string `json:"description"`
	NbnDetails  struct {
		Product string `json:"product"`
		PoiName string `json:"poiName"`
	} `json:"nbnDetails"`
	NextBillDate     time.Time `json:"nextBillDate"`
	OpenDate         string    `json:"openDate"`
	UsageAnniversary int       `json:"usageAnniversary"`
	IPAddresses      []string  `json:"ipAddresses"`
	Address          struct {
		Subaddresstype   string `json:"subaddresstype"`
		Subaddressnumber string `json:"subaddressnumber"`
		Streetnumber     string `json:"streetnumber"`
		Streetname       string `json:"streetname"`
		Streettype       string `json:"streettype"`
		Locality         string `json:"locality"`
		Postcode         string `json:"postcode"`
		State            string `json:"state"`
	} `json:"address"`
	Contract struct {
		ServiceID       int    `json:"service_id"`
		ContractStart   string `json:"contract_start"`
		ContractLength  int    `json:"contract_length"`
		ContractVersion string `json:"contract_version"`
	} `json:"contract"`
}

//CustomerDetails - https://myaussie-api.aussiebroadband.com.au/customer
type CustomerDetails struct {
	CustomerNumber int    `json:"customer_number"`
	BillingName    string `json:"billing_name"`
	Billformat     int    `json:"billformat"`
	Brand          string `json:"brand"`
	PostalAddress  struct {
		Address  string `json:"address"`
		Town     string `json:"town"`
		State    string `json:"state"`
		Postcode string `json:"postcode"`
	} `json:"postalAddress"`
	CommunicationPreferences struct {
		Outages struct {
			Sms    bool `json:"sms"`
			Sms247 bool `json:"sms247"`
			Email  bool `json:"email"`
		} `json:"outages"`
	} `json:"communicationPreferences"`
	Phone               string   `json:"phone"`
	Email               []string `json:"email"`
	PaymentMethod       string   `json:"payment_method"`
	IsSuspended         bool     `json:"isSuspended"`
	AccountBalanceCents int      `json:"accountBalanceCents"`
	Services            struct {
		NBN []NBNService `json:"NBN"`
	} `json:"services"`
	Permissions struct {
		CreatePaymentPlan          bool `json:"createPaymentPlan"`
		UpdatePaymentDetails       bool `json:"updatePaymentDetails"`
		CreateContact              bool `json:"createContact"`
		UpdateContacts             bool `json:"updateContacts"`
		UpdateCustomer             bool `json:"updateCustomer"`
		ChangePassword             bool `json:"changePassword"`
		CreateTickets              bool `json:"createTickets"`
		MakePayment                bool `json:"makePayment"`
		PurchaseDatablocksNextBill bool `json:"purchaseDatablocksNextBill"`
		CreateOrder                bool `json:"createOrder"`
		ViewOrders                 bool `json:"viewOrders"`
	} `json:"permissions"`
	CreditCard struct {
		NameOnCard string `json:"nameOnCard"`
		Number     string `json:"number"`
		Expiry     string `json:"expiry"`
	} `json:"creditCard"`
}

//UsageInformation - https://myaussie-api.aussiebroadband.com.au/broadband/<sid>/usage
type UsageInformation struct {
	UsedMb        int    `json:"usedMb"`
	DownloadedMb  int    `json:"downloadedMb"`
	UploadedMb    int    `json:"uploadedMb"`
	RemainingMb   *int   `json:"remainingMb"`
	DaysTotal     int    `json:"daysTotal"`
	DaysRemaining int    `json:"daysRemaining"`
	LastUpdated   string `json:"lastUpdated"`
}

//Outages - https://myaussie-api.aussiebroadband.com.au/nbn/<sid>/outages
type Outages struct {
	CurrentNbnOutages   []interface{} `json:"currentNbnOutages"`
	ScheduledNbnOutages []interface{} `json:"scheduledNbnOutages"`
	NetworkEvents       []struct {
		Reference   int         `json:"reference"`
		Title       string      `json:"title"`
		Summary     string      `json:"summary"`
		StartTime   string      `json:"start_time"`
		EndTime     string      `json:"end_time"`
		RestoredAt  interface{} `json:"restored_at"`
		LastUpdated interface{} `json:"last_updated"`
	} `json:"networkEvents"`
}

//GetCustomerDetails get the customer response
func getCustomerDetails(username string, password string, http *httpclient.HttpClient) (*CustomerDetails, error) {
	log.Printf("[MyAussie#%s]Getting My Aussie usage", username)
	url := "https://myaussie-auth.aussiebroadband.com.au/login"

	resp, err := http.Post(url, map[string]string{
		"username": username,
		"password": password,
	})

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	resp, err = http.Get("https://myaussie-api.aussiebroadband.com.au/customer")
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	log.Printf("[MyAussie#%s] response: %s\n", username, string(body))

	data := CustomerDetails{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}

	return &data, nil
}

func getUsage(serviceID int, http *httpclient.HttpClient) (*UsageInformation, error) {
	resp, err := http.Get(fmt.Sprintf("https://myaussie-api.aussiebroadband.com.au/broadband/%d/usage", serviceID))
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	data := UsageInformation{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}

	return &data, nil
}

func parseForUser(username string, password string, influx influxdb.Client) {
	http := httpclient.NewHttpClient()
	customer, err := getCustomerDetails(username, password, http)

	if err != nil {
		log.Fatalf("[MyAussie:%s] ERROR %s\n", username, err)
		return
	}

	points, err := influxdb.NewBatchPoints(influxdb.BatchPointsConfig{
		Database:  os.Getenv("INFLUX_DB"),
		Precision: "s",
	})

	for _, service := range customer.Services.NBN {
		usage, err := getUsage(service.ServiceID, http)
		if err != nil {
			log.Fatalf("[MyAussie:%s] GetUsage ERROR %s\n", username, err)
			continue
		}

		tags := map[string]string{
			"service_id":  strconv.Itoa(service.ServiceID),
			"description": service.Description,
			"poi":         service.NbnDetails.PoiName,
			"product":     service.NbnDetails.Product,
			"rollover":    strconv.Itoa(service.UsageAnniversary),
			"brand":       customer.Brand,
			"user":        username,
		}

		fields := map[string]interface{}{
			"download":       usage.DownloadedMb * 1000 * 1000,
			"upload":         usage.UploadedMb * 1000 * 1000,
			"used":           usage.UsedMb * 1000 * 1000,
			"days_total":     usage.DaysTotal,
			"days_remaining": usage.DaysRemaining,
			"description":    service.Description,
			"poi":            service.NbnDetails.PoiName,
			"product":        service.NbnDetails.Product,
			"rollover":       service.UsageAnniversary,
			// "allowance":    -1,
			// "left":         usage.RemainingMb,
		}

		if usage.RemainingMb != nil {
			fields["left"] = usage.RemainingMb
		}

		if usage.RemainingMb != nil {
			fields["allowance"] = (usage.UsedMb + *usage.RemainingMb) * 1000 * 1000
		}

		t, err := time.ParseInLocation("2006-01-02 15:04:05", usage.LastUpdated, time.Now().Location())
		if err != nil {
			log.Fatalln(err)
			continue
		}

		pt, err := influxdb.NewPoint(
			"usage",
			tags,
			fields,
			t,
		)

		if err != nil {
			log.Fatalln(err)
			continue
		}

		points.AddPoint(pt)
	}
	err = influx.Write(points)
	if err != nil {
		fmt.Printf("[MyAussie:%s] ERROR Writing to influx: %s", username, err)
		return
	}
}
