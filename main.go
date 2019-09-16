package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	aussiebroadband "github.com/Cazzar/go-myaussieapi"
	"github.com/fiorix/go-redis/redis"
	influxdb "github.com/influxdata/influxdb1-client/v2"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load(".env")
	sleepinterval, _ := time.ParseDuration("15m")

	influx, err := influxdb.NewHTTPClient(influxdb.HTTPConfig{
		Addr:     fmt.Sprintf("http://%s:%s", os.Getenv("INFLUX_HOST"), os.Getenv("INFLUX_PORT")),
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

	cache := redis.New(os.Getenv("REDIS_CONN"))

	users := strings.Split(os.Getenv("MYAUSSIE_USER"), ",")
	passwords := strings.Split(os.Getenv("MYAUSSIE_PASS"), ",")

	if len(users) != len(passwords) {
		log.Fatal("Usernames and password lengths do not match at all.")
		return
	}

	customers := make(map[string]*aussiebroadband.Customer)

	for idx, user := range users {
		var cust *aussiebroadband.Customer
		token, err := cache.Get(fmt.Sprintf("aussiebb:%s:token", user))
		dur, _ := cache.TTL(fmt.Sprintf("aussiebb:%s:token", user))
		if err == nil && token != "" && dur <= 0 {
			refreshToken, _ := cache.Get(fmt.Sprintf("aussiebb:%s:refreshtoken", user))
			ttl := time.Now().Add(time.Second * time.Duration(dur))
			cust, err = aussiebroadband.FromToken(user, passwords[idx], token, refreshToken, ttl)
		} else {
			cust, err = aussiebroadband.NewCustomer(user, passwords[idx])
			if err != nil {
				log.Printf("Error getting customer details for: %s, error: %s\n", user, err)
				continue
			}

			cache.Set(fmt.Sprintf("aussiebb:%s:token", cust.Username), cust.Cookie)
			cache.Set(fmt.Sprintf("aussiebb:%s:refreshtoken", cust.Username), cust.RefreshToken)
			cache.Expire(fmt.Sprintf("aussiebb:%s:token", cust.Username), int(cust.ExpiresAt.Sub(time.Now()).Seconds()))
			cache.Expire(fmt.Sprintf("aussiebb:%s:refreshtoken", cust.Username), int(cust.ExpiresAt.Sub(time.Now()).Seconds()))
		}

		customers[user] = cust
	}

	for {
		for _, customer := range customers {
			go parseForUser(customer, influx, cache)
		}
		time.Sleep(sleepinterval)
	}
}

func parseForUser(customer *aussiebroadband.Customer, influx influxdb.Client, cache *redis.Client) {
	refreshed, err := customer.RefreshIfNeeded()
	if refreshed {
		cache.Set(fmt.Sprintf("aussiebb:%s:token", customer.Username), customer.Cookie)
		cache.Set(fmt.Sprintf("aussiebb:%s:refreshtoken", customer.Username), customer.RefreshToken)
		cache.Expire(fmt.Sprintf("aussiebb:%s:token", customer.Username), int(customer.ExpiresAt.Sub(time.Now()).Seconds()))
		cache.Expire(fmt.Sprintf("aussiebb:%s:refreshtoken", customer.Username), int(customer.ExpiresAt.Sub(time.Now()).Seconds()))
	}

	details, err := customer.GetCustomerDetails()

	if err != nil {
		log.Fatalf("[MyAussie:%s] ERROR %s\n", customer.Username, err)
		return
	}

	points, err := influxdb.NewBatchPoints(influxdb.BatchPointsConfig{
		Database:  os.Getenv("INFLUX_DB"),
		Precision: "s",
	})

	for _, service := range details.Services.NBN {
		usage, err := customer.GetUsage(service.ServiceID)
		if err != nil {
			log.Printf("[MyAussie:%s] GetUsage ERROR %s\n", customer.Username, err)
			continue
		}

		tags := map[string]string{
			"service_id":  strconv.Itoa(service.ServiceID),
			"description": service.Description,
			"poi":         service.NbnDetails.PoiName,
			"product":     service.NbnDetails.Product,
			"rollover":    strconv.Itoa(service.UsageAnniversary),
			"brand":       details.Brand,
			"user":        customer.Username,
		}

		fields := map[string]interface{}{
			"download":       usage.DownloadedMb * 1000 * 1000,
			"upload":         usage.UploadedMb * 1000 * 1000,
			"used":           usage.UsedMb * 1000 * 1000,
			"days_total":     usage.DaysTotal,
			"days_remaining": usage.DaysRemaining,
			"description":    service.Description,
			"poi":            service.NbnDetails.PoiName,
			"cvc_graph":      service.NbnDetails.CVCGraph,
			"product":        service.NbnDetails.Product,
			"rollover":       service.UsageAnniversary,
			// "allowance":    -1,
			// "left":         usage.RemainingMb,
		}

		if service.NbnDetails.SpeedPotential != nil {
			time, err := time.Parse(time.RFC3339, service.NbnDetails.SpeedPotential.LastTested)
			if err == nil {
				pt, err := influxdb.NewPoint(
					"speed_potential",
					tags,
					map[string]interface{}{
						"download": service.NbnDetails.SpeedPotential.DownloadMbps,
						"upload":   service.NbnDetails.SpeedPotential.UploadMbps,
					},
					time,
				)
				if err != nil {
					points.AddPoint(pt)
				}
			} else {
				log.Println(err)
			}
		}

		if usage.RemainingMb != nil {
			fields["left"] = *usage.RemainingMb
		}

		if usage.RemainingMb != nil {
			fields["allowance"] = (usage.UsedMb + *usage.RemainingMb) * 1000 * 1000
		}
		if usage.LastUpdated == "" {
			continue
		}

		t, err := time.ParseInLocation("2006-01-02 15:04:05", usage.LastUpdated, time.Now().Location())
		if err != nil {
			log.Println(err)
			continue
		}

		pt, err := influxdb.NewPoint(
			"usage",
			tags,
			fields,
			t,
		)

		if err != nil {
			log.Println(err)
			continue
		}

		points.AddPoint(pt)
	}
	err = influx.Write(points)
	if err != nil {
		fmt.Printf("[MyAussie:%s] ERROR Writing to influx: %s", customer.Username, err)
		return
	}
}
