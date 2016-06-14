package main

import (
        "crypto/hmac"
        "crypto/sha1"
        "crypto/tls"
        "encoding/base64"
        "encoding/json"
        "encoding/xml"
        "io/ioutil"
        "log"
        "net/http"
        "os"
        "os/signal"
        "strings"
        "syscall"
        "time"
)

const (
        L3_KEY_ID         = ""
        L3_SECRET         = ""
        L3_BASEURL        = "https://ws.level3.com"
        REALTIME_ENDPOINT = "/rtm/cdn/v1.0/131992?serviceType=c&geo=none&property=true"
        DATE_LAYOUT       = "Mon, 02 Jan 2006 15:04:05 +0000"
        HTTP_LISTEN       = ":1338"
        UPDATE_INTERVAL   = 20
)

type AccessGroup struct {
        Name     string `xml:"name,attr" json:"name"`
        Time     string `xml:"time" json:"time"`
        Networks []struct {
                NetworkIdentifier string  `xml:"id,attr" json:"name"`
                Mbps              float32 `xml:"mbps" json:"mbps"`
                RequestsPerSecond float32 `xml:"requestsPerSecond" json:"requestsPerSecond"`
                MissMbps          float32 `xml:"missMbps" json:"missMbps"`
                MissPerSecond     float32 `xml:"missPerSecond" json:"missPerSecond"`
                Status403PerSec   float32 `xml:"status403PerSec" json:"status403PerSec"`
                Status404PerSec   float32 `xml:"status404PerSec" json:"status404PerSec"`
                Status503PerSec   float32 `xml:"status503PerSec" json:"status503PerSec"`
                Status504PerSec   float32 `xml:"status504PerSec" json:"status504PerSec"`
                Status5xxPerSec   float32 `xml:"status5xxPerSec" json:"status5xxPerSec"`
                PtPerSec          float32 `xml:"ptPerSec" json:"ptPerSec"`
                HitRatePercentage float32 `xml:"hitRatePercentage" json:"hitRatePercentage"`
        } `xml:"services>service>networkIdentifiers>ni" json:"networks"`
}

var jsonRealtime []byte

func updateRealtime() {
        startTime := time.Now()

        resp, err := l3Get(L3_BASEURL + REALTIME_ENDPOINT)

        if err != nil {
                log.Printf("Error fetching realtime data: %q", err)
        } else {
                defer resp.Body.Close()
                body, err := ioutil.ReadAll(resp.Body)

                if err != nil {
                        log.Printf("Error reading realtime data: %q", err)
                } else {
                        if resp.StatusCode != 200 {
                                log.Printf("Error reading realtime data. Got status '%s'", resp.Status)
                                log.Printf(string(body[:]))
                        } else {
                                jsonRealtime, err = realtimeAsJson(&body)
                                if err != nil {
                                        log.Printf("Error parsing realtime data: %q", err)
                                } else {
                                        log.Printf("Updating realtime took %v", time.Now().Sub(startTime))
                                }
                        }
                }
        }
}

func l3Get(url string) (resp *http.Response, err error) {
        date := time.Now().UTC().Format(DATE_LAYOUT)

        tr := &http.Transport{
                TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
        }

        client := &http.Client{
                Transport: tr,
        }

        req, err := http.NewRequest("GET", url, nil)
        req.Header.Add("Authorization", "MPA "+L3_KEY_ID+":"+l3Sign(date, REALTIME_ENDPOINT))
        req.Header.Add("Date", date)
        req.Header.Add("Content-Type", "text/xml")

        return client.Do(req)
}

func l3Sign(date string, endPoint string) string {
        signInput := date + "\n"
        signInput += strings.Split(endPoint, "?")[0] + "\n"
        signInput += "text/xml\n"
        signInput += "GET\n"

        mac := hmac.New(sha1.New, []byte(L3_SECRET))
        mac.Write([]byte(signInput))

        return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func realtimeAsJson(xmlData *[]byte) ([]byte, error) {
        data := &AccessGroup{}

        err := xml.Unmarshal(*xmlData, data)
        if nil != err {
                log.Printf("Error unmarshalling from XML: %q", err)
                return nil, err
        }

        return json.Marshal(data)
}

func realtimeHandler(w http.ResponseWriter, r *http.Request) {
        log.Printf("Handing realtime request from %s", r.RemoteAddr)
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        w.Write(jsonRealtime)
}

func main() {
        log.Printf("Starting l3api server")

        // Initial realtime update
        updateRealtime()

        // Setup end point
        go func() {
                http.HandleFunc("/realtime", realtimeHandler)
                log.Printf("Handling requests at %s", HTTP_LISTEN)
                http.ListenAndServe(HTTP_LISTEN, nil)
        }()

        // Setup channels and timer
        sigs := make(chan os.Signal, 1)
        msgUpdateChannel := time.NewTicker(time.Second * UPDATE_INTERVAL).C
        doneChan := make(chan bool, 1)

        // Handle shut down signals
        signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

        // Daemon mode
        for {
                select {
                case <-msgUpdateChannel:
                        updateRealtime()
                case sig := <-sigs:
                        log.Printf("Got shut down signal: ", sig)
                        doneChan <- true // Not working… Why?
                case <-doneChan:
                        log.Printf("Shutting down")
                        return
                }
        }

        log.Printf("Stopped l3api server")
}
