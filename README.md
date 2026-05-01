# 🚀 Discord Username Checker (Go)

A fast, multi-mode Discord username checker built in Go.
Designed for high performance with rotating residential proxies and webhook support.

---

## ⚡ Features

* 🔄 **Rotating Residential Proxies** for improved reliability and speed
* 🚀 **High-Speed Checking** — optimized for maximum throughput
* 🎯 **Multiple Modes** to suit different checking strategies
* 📡 **Discord Webhook Integration** for instant notifications
* 🧩 **Simple Setup** — just import proxies and run

---

## 🛠 Requirements

* Go (latest version recommended)
* Working proxies (residential recommended)

Install Go here: https://go.dev/doc/install

---

## 📦 Installation

1. Clone the repository:

   ```bash
   git clone https://github.com/lastseenactive/discordusernamechecker.git
   cd discordusernamechecker
   ```

2. Add your proxies:

   * Place your proxies inside the designated file (e.g. `proxies.txt`)
   * Format depends on the project (IP:PORT or USER:PASS@IP:PORT)

3. Configure settings:

   * Add your Discord webhook URL (if applicable)
   * Adjust modes/settings as needed

---

## ▶️ Usage

Run the program with:

```bash
go run main.go
```

Then follow any prompts shown in the console.

---

## ⚙️ Modes

This checker includes multiple modes for different use cases, such as:

* Fast checking mode
* Filtered/targeted checking
* Other optimized strategies

(Refer to the source or config for specifics)

---

## 📡 Webhook Support

When a valid or desired username is found, the program can send results directly to a Discord webhook.

Make sure to:

* Insert your webhook URL in the config/settings file
* Ensure the webhook is active

---

## ⚠️ Disclaimer

This project is for educational purposes only.
Use responsibly and in accordance with Discord's Terms of Service.

---

## 💡 Notes

* Performance depends heavily on proxy quality
* Residential proxies are strongly recommended
* Running too aggressively may result in rate limits or bans

---

## 🧪 Tips

* Use high-quality proxies for best results
* Adjust thread count if configurable
* Monitor webhook output to track hits

---

---

## ⭐ Contributing

Pull requests and improvements are welcome.

---

## 👤 Author
lastseenactive or <https://discord.com/users/1465414780696264870> on discord
