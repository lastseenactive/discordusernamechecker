package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	WORKERS    = 200
	URL_CHECK  = "https://discord.com/api/v9/unique-username/username-attempt-unauthed"
	PROXY      = "paste proxy here"
	STATS_PORT = "8888"
)

const (
	GREEN = "\033[32m"
	RED   = "\033[31m"
	RESET = "\033[0m"
)

var (
	checked       uint64
	taken         uint64
	available     uint64
	totalToCheck  int
	currentStatus string
	availableList []string
	listMutex     sync.RWMutex

	shouldStop    bool
	controlMutex  sync.RWMutex
	chanClosed    bool
	chanMutex     sync.Mutex

	generatedMap  map[string]bool
	mapMutex      sync.RWMutex
	rateChan      chan struct{}
	currentCheck  *CheckConfig
	lastChecked   string
	lastStatus    string
	lastMutex     sync.RWMutex
	
	webhookURL    string
	webhookMutex  sync.RWMutex
)

type CheckConfig struct {
	Mode      string `json:"mode"`
	MinDigits string `json:"minDigits"`
	Random    bool   `json:"random"`
}

func init() {
	generatedMap = make(map[string]bool)
	rateChan = make(chan struct{}, 5000)
	loadWebhook()

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			for i := 0; i < 5000; i++ {
				select {
				case rateChan <- struct{}{}:
				default:
				}
			}
		}
	}()
}

func loadWebhook() {
	data, err := ioutil.ReadFile("webhook.txt")
	if err == nil {
		webhookMutex.Lock()
		webhookURL = strings.TrimSpace(string(data))
		webhookMutex.Unlock()
	}
}

func saveWebhook(url string) error {
	webhookMutex.Lock()
	webhookURL = url
	webhookMutex.Unlock()
	return ioutil.WriteFile("webhook.txt", []byte(url), 0644)
}

func getChar(index int, letters, numbers []rune) rune {
	if index < 26 {
		return letters[index]
	}
	return numbers[index-26]
}

func countDigits(username string) int {
	count := 0
	for _, c := range username {
		if c >= '0' && c <= '9' {
			count++
		}
	}
	return count
}

func hasDigit(chars ...rune) bool {
	for _, c := range chars {
		if c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}

func hasAtLeastTwoIdentical(username string) bool {
	counts := make(map[rune]int)
	for _, c := range username {
		counts[c]++
		if counts[c] >= 2 {
			return true
		}
	}
	return false
}

func isUnique(username string) bool {
	mapMutex.RLock()
	_, exists := generatedMap[username]
	mapMutex.RUnlock()
	return !exists
}

func markAsGenerated(username string) {
	mapMutex.Lock()
	generatedMap[username] = true
	mapMutex.Unlock()
}

// 4C - 4 caractères avec minDigits minimum de chiffres
func generate4C(minDigits int, useRandom bool) []string {
	var usernames []string
	letters := []rune("abcdefghijklmnopqrstuvwxyz")
	numbers := []rune("0123456789")

	if useRandom {
		chars := append(letters, numbers...)
		totalNeeded := 1679616
		attempts := 0
		maxAttempts := totalNeeded * 3

		for len(usernames) < totalNeeded && attempts < maxAttempts {
			c1 := chars[rand.Intn(36)]
			c2 := chars[rand.Intn(36)]
			c3 := chars[rand.Intn(36)]
			c4 := chars[rand.Intn(36)]
			username := string(c1) + string(c2) + string(c3) + string(c4)

			if countDigits(username) >= minDigits && isUnique(username) {
				usernames = append(usernames, username)
				markAsGenerated(username)
			}
			attempts++
		}
	} else {
		// ORDRE: aaaa, aaab, ..., 000z, ..., 9999
		for i1 := 0; i1 < 36; i1++ {
			for i2 := 0; i2 < 36; i2++ {
				for i3 := 0; i3 < 36; i3++ {
					for i4 := 0; i4 < 36; i4++ {
						c1 := getChar(i1, letters, numbers)
						c2 := getChar(i2, letters, numbers)
						c3 := getChar(i3, letters, numbers)
						c4 := getChar(i4, letters, numbers)

						username := string(c1) + string(c2) + string(c3) + string(c4)
						if countDigits(username) >= minDigits {
							usernames = append(usernames, username)
						}
					}
				}
			}
		}
	}

	return usernames
}

// 4L - Juste aaaa à zzzz (que des lettres, sans repeater)
func generate4L(useRandom bool) []string {
	var usernames []string

	if useRandom {
		letters := []rune("abcdefghijklmnopqrstuvwxyz")
		totalNeeded := 456976
		attempts := 0
		maxAttempts := totalNeeded * 3

		for len(usernames) < totalNeeded && attempts < maxAttempts {
			c1 := letters[rand.Intn(26)]
			c2 := letters[rand.Intn(26)]
			c3 := letters[rand.Intn(26)]
			c4 := letters[rand.Intn(26)]
			username := string(c1) + string(c2) + string(c3) + string(c4)

			if isUnique(username) {
				usernames = append(usernames, username)
				markAsGenerated(username)
			}
			attempts++
		}
	} else {
		// ORDRE: aaaa, aaab, aaac, ..., zzzz
		for i1 := 'a'; i1 <= 'z'; i1++ {
			for i2 := 'a'; i2 <= 'z'; i2++ {
				for i3 := 'a'; i3 <= 'z'; i3++ {
					for i4 := 'a'; i4 <= 'z'; i4++ {
						username := string(i1) + string(i2) + string(i3) + string(i4)
						usernames = append(usernames, username)
					}
				}
			}
		}
	}

	return usernames
}

// Semi 3L - xxx. et xxx_ (SANS chiffres)
func generateSemi3L() []string {
	var usernames []string

	// ORDRE: aaa., aab., ..., zzz.
	for i1 := 'a'; i1 <= 'z'; i1++ {
		for i2 := 'a'; i2 <= 'z'; i2++ {
			for i3 := 'a'; i3 <= 'z'; i3++ {
				usernames = append(usernames, string(i1)+string(i2)+string(i3)+".")
			}
		}
	}

	// ORDRE: aaa_, aab_, ..., zzz_
	for i1 := 'a'; i1 <= 'z'; i1++ {
		for i2 := 'a'; i2 <= 'z'; i2++ {
			for i3 := 'a'; i3 <= 'z'; i3++ {
				usernames = append(usernames, string(i1)+string(i2)+string(i3)+"_")
			}
		}
	}

	return usernames
}

// 4N - 1000 à 9999
func generate4N() []string {
	var usernames []string
	for i := 1000; i <= 9999; i++ {
		usernames = append(usernames, strconv.Itoa(i))
	}
	return usernames
}

// Semi 4N - 0000. à 9999. et 0000_ à 9999_
func generateSemi4N() []string {
	var usernames []string

	// ORDRE: 0000., 0001., ..., 9999.
	for i := 0; i <= 9999; i++ {
		usernames = append(usernames, fmt.Sprintf("%04d.", i))
	}

	// ORDRE: 0000_, 0001_, ..., 9999_
	for i := 0; i <= 9999; i++ {
		usernames = append(usernames, fmt.Sprintf("%04d_", i))
	}

	return usernames
}

// Semi 3C - xxx. et xxx_ (avec AU MOINS 1 chiffre)
func generateSemi3C() []string {
	var usernames []string
	letters := []rune("abcdefghijklmnopqrstuvwxyz")
	numbers := []rune("0123456789")

	// ORDRE: 000., 001., ..., aaa., ..., zzz. (que ceux avec au moins 1 chiffre)
	for i1 := 0; i1 < 36; i1++ {
		for i2 := 0; i2 < 36; i2++ {
			for i3 := 0; i3 < 36; i3++ {
				c1 := getChar(i1, letters, numbers)
				c2 := getChar(i2, letters, numbers)
				c3 := getChar(i3, letters, numbers)

				if hasDigit(c1, c2, c3) {
					usernames = append(usernames, string(c1)+string(c2)+string(c3)+".")
				}
			}
		}
	}

	// ORDRE: 000_, 001_, ..., aaa_, ..., zzz_ (que ceux avec au moins 1 chiffre)
	for i1 := 0; i1 < 36; i1++ {
		for i2 := 0; i2 < 36; i2++ {
			for i3 := 0; i3 < 36; i3++ {
				c1 := getChar(i1, letters, numbers)
				c2 := getChar(i2, letters, numbers)
				c3 := getChar(i3, letters, numbers)

				if hasDigit(c1, c2, c3) {
					usernames = append(usernames, string(c1)+string(c2)+string(c3)+"_")
				}
			}
		}
	}

	return usernames
}

// 4L Repeater - Au moins 2 lettres identiques
func generate4LRepeater(useRandom bool) []string {
	var usernames []string

	if useRandom {
		letters := []rune("abcdefghijklmnopqrstuvwxyz")
		totalNeeded := 0

		for i1 := 'a'; i1 <= 'z'; i1++ {
			for i2 := 'a'; i2 <= 'z'; i2++ {
				for i3 := 'a'; i3 <= 'z'; i3++ {
					for i4 := 'a'; i4 <= 'z'; i4++ {
						username := string(i1) + string(i2) + string(i3) + string(i4)
						if hasAtLeastTwoIdentical(username) {
							totalNeeded++
						}
					}
				}
			}
		}

		attempts := 0
		maxAttempts := totalNeeded * 3

		for len(usernames) < totalNeeded && attempts < maxAttempts {
			c1 := letters[rand.Intn(26)]
			c2 := letters[rand.Intn(26)]
			c3 := letters[rand.Intn(26)]
			c4 := letters[rand.Intn(26)]
			username := string(c1) + string(c2) + string(c3) + string(c4)

			if hasAtLeastTwoIdentical(username) && isUnique(username) {
				usernames = append(usernames, username)
				markAsGenerated(username)
			}
			attempts++
		}
	} else {
		// ORDRE: aaaa, aaab, ..., (skip abcd), ..., zzzz
		for i1 := 'a'; i1 <= 'z'; i1++ {
			for i2 := 'a'; i2 <= 'z'; i2++ {
				for i3 := 'a'; i3 <= 'z'; i3++ {
					for i4 := 'a'; i4 <= 'z'; i4++ {
						username := string(i1) + string(i2) + string(i3) + string(i4)
						if hasAtLeastTwoIdentical(username) {
							usernames = append(usernames, username)
						}
					}
				}
			}
		}
	}

	return usernames
}

// Bad Semi 3L - x.xx, xx.x, _xxx, x_xx, xx_x, xxx_ (SANS chiffres)
func generateBadSemi3L() []string {
	var usernames []string

	// x.xx - ORDRE: a.aa, a.ab, ..., z.zz
	for i1 := 'a'; i1 <= 'z'; i1++ {
		for i2 := 'a'; i2 <= 'z'; i2++ {
			for i3 := 'a'; i3 <= 'z'; i3++ {
				usernames = append(usernames, string(i1)+"."+string(i2)+string(i3))
			}
		}
	}

	// xx.x - ORDRE: aa.a, aa.b, ..., zz.z
	for i1 := 'a'; i1 <= 'z'; i1++ {
		for i2 := 'a'; i2 <= 'z'; i2++ {
			for i3 := 'a'; i3 <= 'z'; i3++ {
				usernames = append(usernames, string(i1)+string(i2)+"."+string(i3))
			}
		}
	}

	// _xxx - ORDRE: _aaa, _aab, ..., _zzz
	for i1 := 'a'; i1 <= 'z'; i1++ {
		for i2 := 'a'; i2 <= 'z'; i2++ {
			for i3 := 'a'; i3 <= 'z'; i3++ {
				usernames = append(usernames, "_"+string(i1)+string(i2)+string(i3))
			}
		}
	}

	// x_xx - ORDRE: a_aa, a_ab, ..., z_zz
	for i1 := 'a'; i1 <= 'z'; i1++ {
		for i2 := 'a'; i2 <= 'z'; i2++ {
			for i3 := 'a'; i3 <= 'z'; i3++ {
				usernames = append(usernames, string(i1)+"_"+string(i2)+string(i3))
			}
		}
	}

	// xx_x - ORDRE: aa_a, aa_b, ..., zz_z
	for i1 := 'a'; i1 <= 'z'; i1++ {
		for i2 := 'a'; i2 <= 'z'; i2++ {
			for i3 := 'a'; i3 <= 'z'; i3++ {
				usernames = append(usernames, string(i1)+string(i2)+"_"+string(i3))
			}
		}
	}

	// xxx_ - ORDRE: aaa_, aab_, ..., zzz_
	for i1 := 'a'; i1 <= 'z'; i1++ {
		for i2 := 'a'; i2 <= 'z'; i2++ {
			for i3 := 'a'; i3 <= 'z'; i3++ {
				usernames = append(usernames, string(i1)+string(i2)+string(i3)+"_")
			}
		}
	}

	return usernames
}

// Bad Semi 3C - x.xx, xx.x, _xxx, x_xx, xx_x, xxx_ (avec AU MOINS 1 chiffre)
func generateBadSemi3C() []string {
	var usernames []string
	letters := []rune("abcdefghijklmnopqrstuvwxyz")
	numbers := []rune("0123456789")

	// x.xx - ORDRE: 0.00, 0.01, ..., z.zz (que ceux avec au moins 1 chiffre)
	for i1 := 0; i1 < 36; i1++ {
		for i2 := 0; i2 < 36; i2++ {
			for i3 := 0; i3 < 36; i3++ {
				c1 := getChar(i1, letters, numbers)
				c2 := getChar(i2, letters, numbers)
				c3 := getChar(i3, letters, numbers)
				username := string(c1) + "." + string(c2) + string(c3)
				if countDigits(username) >= 1 {
					usernames = append(usernames, username)
				}
			}
		}
	}

	// xx.x - ORDRE: 00.0, 00.1, ..., zz.z (que ceux avec au moins 1 chiffre)
	for i1 := 0; i1 < 36; i1++ {
		for i2 := 0; i2 < 36; i2++ {
			for i3 := 0; i3 < 36; i3++ {
				c1 := getChar(i1, letters, numbers)
				c2 := getChar(i2, letters, numbers)
				c3 := getChar(i3, letters, numbers)
				username := string(c1) + string(c2) + "." + string(c3)
				if countDigits(username) >= 1 {
					usernames = append(usernames, username)
				}
			}
		}
	}

	// _xxx - ORDRE: _000, _001, ..., _zzz (que ceux avec au moins 1 chiffre)
	for i1 := 0; i1 < 36; i1++ {
		for i2 := 0; i2 < 36; i2++ {
			for i3 := 0; i3 < 36; i3++ {
				c1 := getChar(i1, letters, numbers)
				c2 := getChar(i2, letters, numbers)
				c3 := getChar(i3, letters, numbers)
				username := "_" + string(c1) + string(c2) + string(c3)
				if countDigits(username) >= 1 {
					usernames = append(usernames, username)
				}
			}
		}
	}

	// x_xx - ORDRE: 0_00, 0_01, ..., z_zz (que ceux avec au moins 1 chiffre)
	for i1 := 0; i1 < 36; i1++ {
		for i2 := 0; i2 < 36; i2++ {
			for i3 := 0; i3 < 36; i3++ {
				c1 := getChar(i1, letters, numbers)
				c2 := getChar(i2, letters, numbers)
				c3 := getChar(i3, letters, numbers)
				username := string(c1) + "_" + string(c2) + string(c3)
				if countDigits(username) >= 1 {
					usernames = append(usernames, username)
				}
			}
		}
	}

	// xx_x - ORDRE: 00_0, 00_1, ..., zz_z (que ceux avec au moins 1 chiffre)
	for i1 := 0; i1 < 36; i1++ {
		for i2 := 0; i2 < 36; i2++ {
			for i3 := 0; i3 < 36; i3++ {
				c1 := getChar(i1, letters, numbers)
				c2 := getChar(i2, letters, numbers)
				c3 := getChar(i3, letters, numbers)
				username := string(c1) + string(c2) + "_" + string(c3)
				if countDigits(username) >= 1 {
					usernames = append(usernames, username)
				}
			}
		}
	}

	// xxx_ - ORDRE: 000_, 001_, ..., zzz_ (que ceux avec au moins 1 chiffre)
	for i1 := 0; i1 < 36; i1++ {
		for i2 := 0; i2 < 36; i2++ {
			for i3 := 0; i3 < 36; i3++ {
				c1 := getChar(i1, letters, numbers)
				c2 := getChar(i2, letters, numbers)
				c3 := getChar(i3, letters, numbers)
				username := string(c1) + string(c2) + string(c3) + "_"
				if countDigits(username) >= 1 {
					usernames = append(usernames, username)
				}
			}
		}
	}

	return usernames
}

// xxx.
func generateSemi3LDot() []string {
	var usernames []string

	for i1 := 'a'; i1 <= 'z'; i1++ {
		for i2 := 'a'; i2 <= 'z'; i2++ {
			for i3 := 'a'; i3 <= 'z'; i3++ {
				usernames = append(usernames, string(i1)+string(i2)+string(i3)+".")
			}
		}
	}

	return usernames
}

// xxx_
func generateSemi3LUnderscore() []string {
	var usernames []string

	for i1 := 'a'; i1 <= 'z'; i1++ {
		for i2 := 'a'; i2 <= 'z'; i2++ {
			for i3 := 'a'; i3 <= 'z'; i3++ {
				usernames = append(usernames, string(i1)+string(i2)+string(i3)+"_")
			}
		}
	}

	return usernames
}

func loadFromTxt() ([]string, error) {
	file, err := os.Open("usernames.txt")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var usernames []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		username := strings.TrimSpace(scanner.Text())
		if username != "" {
			usernames = append(usernames, username)
		}
	}

	return usernames, scanner.Err()
}

func setLastChecked(username, status string) {
	lastMutex.Lock()
	lastChecked = username
	lastStatus = status
	lastMutex.Unlock()
}

func logLine(status, username string) {
	color := RED
	if status == "AVAILABLE" {
		color = GREEN
	}
	fmt.Printf("%s[%s] %s%s\n", color, status, username, RESET)
	setLastChecked(username, status)
}

func saveToFile(username string) {
	listMutex.Lock()
	availableList = append(availableList, username)
	listMutex.Unlock()

	f, err := os.OpenFile("available.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(username + "\n")
}

func sendWebhook(message string) {
	webhookMutex.RLock()
	webhook := webhookURL
	webhookMutex.RUnlock()

	if webhook == "" {
		return
	}

	if message == "Start" {
		payload := map[string]interface{}{
			"embeds": []map[string]interface{}{
				{
					"title":       "Check Started",
					"description": fmt.Sprintf("Mode: %s", currentCheck.Mode),
					"color":       65280,
				},
			},
		}
		sendWebhookPayload(payload, webhook)
	} else if message == "End" {
		payload := map[string]interface{}{
			"embeds": []map[string]interface{}{
				{
					"title": "Check Completed",
					"description": fmt.Sprintf("Checked: %d\nAvailable: %d\nTaken: %d",
						atomic.LoadUint64(&checked),
						atomic.LoadUint64(&available),
						atomic.LoadUint64(&taken)),
					"color": 255,
				},
			},
		}
		sendWebhookPayload(payload, webhook)
	} else {
		payload := map[string]interface{}{
			"content": message,
		}
		sendWebhookPayload(payload, webhook)
	}
}

func sendWebhookPayload(payload map[string]interface{}, webhook string) {
	data, _ := json.Marshal(payload)
	http.Post(webhook, "application/json", bytes.NewBuffer(data))
}

func startHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var config CheckConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	atomic.StoreUint64(&checked, 0)
	atomic.StoreUint64(&taken, 0)
	atomic.StoreUint64(&available, 0)

	controlMutex.Lock()
	shouldStop = false
	controlMutex.Unlock()

	lastMutex.Lock()
	lastChecked = ""
	lastStatus = ""
	lastMutex.Unlock()

	currentCheck = &config
	currentStatus = "CHECKING"

	go runCheck(config)

	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func stopHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	controlMutex.Lock()
	shouldStop = true
	controlMutex.Unlock()
	currentStatus = "STOPPED"

	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		webhookMutex.RLock()
		webhook := webhookURL
		webhookMutex.RUnlock()
		json.NewEncoder(w).Encode(map[string]string{"webhook": webhook})
	} else if r.Method == http.MethodPost {
		var data map[string]string
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if err := saveWebhook(data["webhook"]); err != nil {
			http.Error(w, "Failed to save", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}
}

func statsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	percentage := 0.0
	if totalToCheck > 0 {
		percentage = float64(atomic.LoadUint64(&checked)) / float64(totalToCheck) * 100
	}

	listMutex.RLock()
	availableCopy := make([]string, len(availableList))
	copy(availableCopy, availableList)
	listMutex.RUnlock()

	lastMutex.RLock()
	lastU := lastChecked
	lastS := lastStatus
	lastMutex.RUnlock()

	stats := map[string]interface{}{
		"checked":       atomic.LoadUint64(&checked),
		"taken":         atomic.LoadUint64(&taken),
		"available":     atomic.LoadUint64(&available),
		"total":         totalToCheck,
		"percentage":    percentage,
		"status":        currentStatus,
		"availableList": availableCopy,
		"lastChecked":   lastU,
		"lastStatus":    lastS,
	}

	json.NewEncoder(w).Encode(stats)
}

func htmlHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Discord Checker</title>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: 'Inter', -apple-system, sans-serif; background: #000; color: #fff; padding: 30px 15px; }
        .container { max-width: 1200px; margin: 0 auto; }
        h1 { font-size: 1.5rem; font-weight: 700; margin-bottom: 30px; text-align: center; }
        .main-grid { display: grid; grid-template-columns: 1fr 400px; gap: 20px; }
        .left-col { display: flex; flex-direction: column; gap: 20px; }
        .card { background: #111; border: 1px solid #222; border-radius: 8px; padding: 25px; }
        .modes { display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; margin-bottom: 20px; }
        .mode { background: #1a1a1a; border: 1px solid #2a2a2a; padding: 14px 8px; text-align: center; border-radius: 6px; cursor: pointer; transition: all 0.2s; font-size: 0.85rem; font-weight: 600; }
        .mode:hover { border-color: #444; }
        .mode.active { background: #fff; color: #000; border-color: #fff; }
        .opts { display: none; padding: 15px; background: #0a0a0a; border-radius: 6px; margin-bottom: 20px; gap: 15px; align-items: center; }
        .opts.show { display: flex; }
        select { background: #1a1a1a; color: #fff; border: 1px solid #2a2a2a; padding: 8px 12px; border-radius: 5px; font-family: 'Inter', sans-serif; font-weight: 500; }
        input[type="text"] { background: #1a1a1a; color: #fff; border: 1px solid #2a2a2a; padding: 8px 12px; border-radius: 5px; font-family: 'Inter', sans-serif; flex: 1; }
        button { background: #fff; color: #000; border: none; padding: 14px; border-radius: 6px; font-size: 0.9rem; font-weight: 700; cursor: pointer; transition: opacity 0.2s; font-family: 'Inter', sans-serif; }
        button:hover:not(:disabled) { opacity: 0.85; }
        button:disabled { opacity: 0.3; cursor: not-allowed; }
        .webhook-input { display: flex; gap: 10px; margin-bottom: 15px; }
        .ctrls { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
        .ctrls button { background: #1a1a1a; color: #fff; border: 1px solid #2a2a2a; }
        .ctrls button:hover:not(:disabled) { border-color: #444; }
        .stats { display: grid; grid-template-columns: repeat(3, 1fr); gap: 15px; }
        .stat { background: #0a0a0a; padding: 18px; border-radius: 6px; text-align: center; }
        .stat-val { font-size: 2rem; font-weight: 700; margin-bottom: 6px; transition: transform 0.2s; }
        .stat-val.pop { transform: scale(1.1); }
        .stat-lbl { color: #666; font-size: 0.75rem; text-transform: uppercase; font-weight: 600; letter-spacing: 0.5px; }
        .bar { height: 4px; background: #1a1a1a; border-radius: 2px; overflow: hidden; margin: 18px 0; }
        .bar-fill { height: 100%; background: #fff; transition: width 0.5s ease; }
        .status { display: inline-block; padding: 4px 12px; background: #1a1a1a; border-radius: 4px; font-size: 0.75rem; text-transform: uppercase; margin-bottom: 15px; transition: all 0.3s; font-weight: 700; }
        .status.active { background: #4ade80; color: #000; }
        .progress-info { display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; }
        .progress-percent { font-size: 1.5rem; font-weight: 700; color: #fff; }
        .live-check { background: #111; border: 1px solid #222; border-radius: 8px; padding: 25px; height: fit-content; }
        .live-title { font-size: 0.8rem; color: #666; text-transform: uppercase; font-weight: 700; letter-spacing: 0.5px; margin-bottom: 20px; }
        .current-check { background: #0a0a0a; padding: 20px; border-radius: 6px; text-align: center; min-height: 200px; display: flex; align-items: center; justify-content: center; }
        .check-username { font-size: 2.5rem; font-weight: 700; font-family: 'Courier New', monospace; margin-bottom: 15px; transition: all 0.3s; }
        .check-username.available { color: #4ade80; }
        .check-username.taken { color: #ef4444; }
        .check-status { font-size: 0.9rem; text-transform: uppercase; font-weight: 700; letter-spacing: 1px; }
        .check-idle { color: #444; font-size: 1rem; font-weight: 600; }
        .list { max-height: 300px; overflow-y: auto; padding: 15px; background: #0a0a0a; border-radius: 6px; }
        .username { color: #4ade80; font-family: 'Courier New', monospace; padding: 8px 0; border-bottom: 1px solid #111; font-weight: 600; }
        .username:last-child { border-bottom: none; }
        .empty { color: #444; text-align: center; padding: 35px; font-size: 0.85rem; font-weight: 500; }
        @media (max-width: 1024px) { .main-grid { grid-template-columns: 1fr; } .modes { grid-template-columns: repeat(2, 1fr); } }
    </style>
</head>
<body>
    <div class="container">
        <h1>Discord Username Checker</h1>
        <div class="main-grid">
            <div class="left-col">
                <div class="card">
                    <div class="webhook-input">
                        <input type="text" id="webhook-url" placeholder="Discord Webhook URL">
                        <button onclick="saveWebhook()">Save</button>
                    </div>
                    <div class="modes">
                        <div class="mode active" data-mode="4c">4C</div>
                        <div class="mode" data-mode="4l">4L</div>
                        <div class="mode" data-mode="semi3l">Semi 3L</div>
                        <div class="mode" data-mode="4n">4N</div>
                        <div class="mode" data-mode="semi4n">Semi 4N</div>
                        <div class="mode" data-mode="semi3c">Semi 3C</div>
                        <div class="mode" data-mode="4lr">4L Repeater</div>
                        <div class="mode" data-mode="badsemi3l">Bad Semi 3L</div>
                        <div class="mode" data-mode="badsemi3c">Bad Semi 3C</div>
                        <div class="mode" data-mode="xxxdot">xxx.</div>
                        <div class="mode" data-mode="xxxunderscore">xxx_</div>
                        <div class="mode" data-mode="viatxt">Via TXT</div>
                    </div>
                    <div class="opts show" id="opt-4c">
                        <label>Min Digits:</label>
                        <select id="digits">
                            <option value="0">0</option>
                            <option value="1" selected>1</option>
                            <option value="2">2</option>
                            <option value="3">3</option>
                        </select>
                        <label><input type="checkbox" id="rand-4c"> Random</label>
                    </div>
                    <div class="opts" id="opt-4l"><label><input type="checkbox" id="rand-4l"> Random</label></div>
                    <div class="opts" id="opt-4lr"><label><input type="checkbox" id="rand-4lr"> Random</label></div>
                    <div class="ctrls">
                        <button id="start" onclick="start()">START</button>
                        <button id="stop" onclick="stop()" disabled>STOP</button>
                    </div>
                </div>
                <div class="card">
                    <div class="progress-info">
                        <span class="status" id="status">Idle</span>
                        <span class="progress-percent" id="percent">0%</span>
                    </div>
                    <div class="bar"><div class="bar-fill" id="bar"></div></div>
                    <div class="stats">
                        <div class="stat"><div class="stat-val" id="checked">0</div><div class="stat-lbl">Checked</div></div>
                        <div class="stat"><div class="stat-val" id="available">0</div><div class="stat-lbl">Available</div></div>
                        <div class="stat"><div class="stat-val" id="taken">0</div><div class="stat-lbl">Taken</div></div>
                    </div>
                </div>
                <div class="card">
                    <div class="list" id="list"><div class="empty">No usernames yet</div></div>
                </div>
            </div>
            <div class="live-check">
                <div class="live-title">Live Checking</div>
                <div class="current-check">
                    <div id="live-content"><div class="check-idle">Waiting to start...</div></div>
                </div>
            </div>
        </div>
    </div>
    <script>
        let mode = '4c', checking = false, prevChecked = 0, prevAvail = 0, prevTaken = 0;
        
        async function loadWebhook() {
            const res = await fetch('/webhook');
            const data = await res.json();
            document.getElementById('webhook-url').value = data.webhook || '';
        }
        
        async function saveWebhook() {
            const webhook = document.getElementById('webhook-url').value;
            await fetch('/webhook', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ webhook })
            });
        }
        
        document.querySelectorAll('.mode').forEach(btn => {
            btn.onclick = () => {
                if (checking) return;
                document.querySelectorAll('.mode').forEach(b => b.classList.remove('active'));
                btn.classList.add('active');
                mode = btn.dataset.mode;
                document.querySelectorAll('.opts').forEach(o => o.classList.remove('show'));
                const opt = document.getElementById('opt-' + mode);
                if (opt) opt.classList.add('show');
            };
        });
        
        async function start() {
            const config = { mode, minDigits: document.getElementById('digits')?.value || '1', random: false };
            if (mode === '4c') config.random = document.getElementById('rand-4c').checked;
            if (mode === '4l') config.random = document.getElementById('rand-4l').checked;
            if (mode === '4lr') config.random = document.getElementById('rand-4lr').checked;
            const res = await fetch('/start', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(config) });
            if (res.ok) { checking = true; updateButtons(); }
        }
        
        async function stop() {
            await fetch('/stop', { method: 'POST' });
            checking = false; updateButtons();
        }
        
        function updateButtons() {
            document.getElementById('start').disabled = checking;
            document.getElementById('stop').disabled = !checking;
            document.querySelectorAll('.mode').forEach(m => m.style.pointerEvents = checking ? 'none' : 'auto');
        }
        
        function animateValue(id, newVal) {
            const el = document.getElementById(id);
            el.classList.add('pop');
            el.textContent = newVal;
            setTimeout(() => el.classList.remove('pop'), 200);
        }
        
        async function update() {
            try {
                const res = await fetch('/stats');
                const data = await res.json();
                if (data.checked !== prevChecked) { animateValue('checked', data.checked || 0); prevChecked = data.checked; }
                if (data.available !== prevAvail) { animateValue('available', data.available || 0); prevAvail = data.available; }
                if (data.taken !== prevTaken) { animateValue('taken', data.taken || 0); prevTaken = data.taken; }
                const percentage = data.percentage || 0;
                document.getElementById('bar').style.width = percentage + '%';
                document.getElementById('percent').textContent = Math.round(percentage) + '%';
                const status = document.getElementById('status');
                status.textContent = data.status || 'Idle';
                status.classList.toggle('active', data.status === 'CHECKING');
                if (data.status === 'COMPLETED' || data.status === 'STOPPED') { checking = false; updateButtons(); }
                const liveContent = document.getElementById('live-content');
                if (data.lastChecked && data.status === 'CHECKING') {
                    const statusClass = data.lastStatus === 'AVAILABLE' ? 'available' : 'taken';
                    const statusText = data.lastStatus === 'AVAILABLE' ? 'AVAILABLE' : 'TAKEN';
                    liveContent.innerHTML = '<div class="check-username ' + statusClass + '">' + data.lastChecked + '</div><div class="check-status">' + statusText + '</div>';
                } else if (data.status === 'COMPLETED') {
                    liveContent.innerHTML = '<div class="check-idle">Check completed!</div>';
                } else if (data.status === 'STOPPED') {
                    liveContent.innerHTML = '<div class="check-idle">Stopped</div>';
                } else {
                    liveContent.innerHTML = '<div class="check-idle">Waiting to start...</div>';
                }
                const list = document.getElementById('list');
                if (data.availableList && data.availableList.length > 0) {
                    list.innerHTML = data.availableList.map(u => '<div class="username">' + u + '</div>').join('');
                } else {
                    list.innerHTML = '<div class="empty">No usernames yet</div>';
                }
            } catch (e) {}
        }
        
        loadWebhook();
        setInterval(update, 100);
        update();
    </script>
</body>
</html>`
	w.Write([]byte(html))
}

func serveWeb() {
	http.HandleFunc("/stats", statsHandler)
	http.HandleFunc("/start", startHandler)
	http.HandleFunc("/stop", stopHandler)
	http.HandleFunc("/webhook", webhookHandler)
	http.HandleFunc("/", htmlHandler)

	fmt.Printf("Web: http://localhost:%s\n", STATS_PORT)

	go func() {
		if err := http.ListenAndServe(":"+STATS_PORT, nil); err != nil {
			fmt.Printf("Server error: %v\n", err)
		}
	}()
}

func runCheck(config CheckConfig) {
	var usernames []string
	var err error

	mapMutex.Lock()
	generatedMap = make(map[string]bool)
	mapMutex.Unlock()

	listMutex.Lock()
	availableList = []string{}
	listMutex.Unlock()

	switch config.Mode {
	case "4c":
		minDigits, _ := strconv.Atoi(config.MinDigits)
		usernames = generate4C(minDigits, config.Random)
	case "4l":
		usernames = generate4L(config.Random)
	case "semi3l":
		usernames = generateSemi3L()
	case "4n":
		usernames = generate4N()
	case "semi4n":
		usernames = generateSemi4N()
	case "semi3c":
		usernames = generateSemi3C()
	case "4lr":
		usernames = generate4LRepeater(config.Random)
	case "badsemi3l":
		usernames = generateBadSemi3L()
	case "badsemi3c":
		usernames = generateBadSemi3C()
	case "xxxdot":
		usernames = generateSemi3LDot()
	case "xxxunderscore":
		usernames = generateSemi3LUnderscore()
	case "viatxt":
		usernames, err = loadFromTxt()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			currentStatus = "ERROR"
			return
		}
	}

	totalToCheck = len(usernames)
	fmt.Printf("Loaded: %d usernames\n", totalToCheck)

	sendWebhook("Start")

	proxyURL, _ := url.Parse(PROXY)
	transport := &http.Transport{
		Proxy:               http.ProxyURL(proxyURL),
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		MaxIdleConns:        2000,
		MaxIdleConnsPerHost: 2000,
		IdleConnTimeout:     60 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   6 * time.Second,
	}

	usernameChan := make(chan string, totalToCheck)
	chanMutex.Lock()
	chanClosed = false
	chanMutex.Unlock()

	for _, username := range usernames {
		usernameChan <- username
	}

	var wg sync.WaitGroup
	for i := 0; i < WORKERS; i++ {
		wg.Add(1)
		go worker(client, usernameChan, &wg)
	}

	wg.Wait()

	chanMutex.Lock()
	if !chanClosed {
		close(usernameChan)
		chanClosed = true
	}
	chanMutex.Unlock()

	if !shouldStop {
		currentStatus = "COMPLETED"
		sendWebhook("End")
	}

	fmt.Printf("\nCompleted: %d checked, %d available, %d taken\n",
		atomic.LoadUint64(&checked),
		atomic.LoadUint64(&available),
		atomic.LoadUint64(&taken))
}

func worker(client *http.Client, usernameChan chan string, wg *sync.WaitGroup) {
	defer wg.Done()

	for username := range usernameChan {
		controlMutex.RLock()
		stop := shouldStop
		controlMutex.RUnlock()

		if stop {
			return
		}

		<-rateChan

		body, _ := json.Marshal(map[string]string{"username": username})
		req, _ := http.NewRequest("POST", URL_CHECK, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

		resp, err := client.Do(req)
		if err != nil {
			chanMutex.Lock()
			closed := chanClosed
			chanMutex.Unlock()

			if !closed {
				select {
				case usernameChan <- username:
				default:
				}
			}
			continue
		}

		func() {
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				chanMutex.Lock()
				closed := chanClosed
				chanMutex.Unlock()

				if !closed {
					select {
					case usernameChan <- username:
					default:
					}
				}
				return
			}

			var res struct {
				Taken bool `json:"taken"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
				chanMutex.Lock()
				closed := chanClosed
				chanMutex.Unlock()

				if !closed {
					select {
					case usernameChan <- username:
					default:
					}
				}
				return
			}

			atomic.AddUint64(&checked, 1)

			if res.Taken {
				atomic.AddUint64(&taken, 1)
				logLine("taken", username)
			} else {
				atomic.AddUint64(&available, 1)
				logLine("AVAILABLE", username)
				saveToFile(username)
				sendWebhook(fmt.Sprintf(" `%s`", username))
			}
		}()
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())

	serveWeb()

	fmt.Println("\nDiscord Username Checker")
	fmt.Println(strings.Repeat("-", 40))
	fmt.Println("Web: http://localhost:" + STATS_PORT)
	fmt.Println(strings.Repeat("-", 40))

	select {}
}