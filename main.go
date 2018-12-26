package main

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	httpclient "github.com/ddliu/go-httpclient"
	influxdb "github.com/influxdata/influxdb/client/v2"
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

	//	user, password := os.Getenv(""), "d4D9jXBsT9F8"
	users := strings.Split(os.Getenv("MYAUSSIE_USER"), ",")
	passwords := strings.Split(os.Getenv("MYAUSSIE_PASS"), ",")

	if len(users) != len(passwords) {
		log.Fatal("Usernames and password lengths do not match at all.")
		return
	}

	for {
		for idx, user := range users {
			res, err := GetFromMyAussie(user, passwords[idx])
			if err != nil {
				log.Fatalln(err)
				continue
			}

			tags := map[string]string{"user": user}
			fields := map[string]interface{}{
				"download":  res.Down1,
				"upload":    res.Up1,
				"allowance": res.Allowance1MB * 1000 * 1000,
				"left":      res.Left1,
				"rollover":  res.Rollover,
			}

			t, err := time.ParseInLocation("2006-01-02 15:04:05", res.LastUpdated, time.Now().Location())
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

			bp, err := influxdb.NewBatchPoints(influxdb.BatchPointsConfig{
				Database:  "aussiebb",
				Precision: "s",
			})

			bp.AddPoint(pt)

			if err := influx.Write(bp); err != nil {
				log.Fatalln(err)
				continue
			}

      log.Printf("[%s] Submitted usage to InfluxDB\n", user)

			time.Sleep(sleepinterval)
		}
	}
}

//Result for GetFromMyAussie
type Result struct {
	XMLName      xml.Name `xml:"usage"`
	Down1        uint     `xml:"down1"`
	Up1          uint     `xml:"up1"`
	Allowance1MB uint     `xml:"allowance1_mb"`
	Left1        uint     `xml:"left1"`
	Down2        uint     `xml:"down2"`
	Up2          uint     `xml:"up2"`
	Left2        uint     `xml:"left2"`
	Allowance2MB uint     `xml:"allowance2_mb"`
	LastUpdated  string   `xml:"lastupdated"`
	Rollover     uint     `xml:"rollover"`
}

//GetFromMyAussie is for pulling the detials from MyAussie.
func GetFromMyAussie(username string, password string) (*Result, error) {
  log.Printf("[MyAussie#%s]Getting My Aussie usage", username)
	url := "https://my.aussiebroadband.com.au/usage.php?xml=yes"

	resp, err := httpclient.Post(url, map[string]string{
		"login_username": username,
		"login_password": password,
	})

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
  if err != nil {
    return nil, err
  }

  log.Print("[MyAussie#%s] XML response: %s\n", username, string(body))

	data := Result{}
	err = xml.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}

	return &data, nil
}
