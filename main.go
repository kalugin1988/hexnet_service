// hexnet_service
// HTTP-сервис с веб-страницей, где можно вводить target/route и hex-строки для конвертации в обе стороны.
// Поддерживает множественные записи в hex-потоке.

package main

import (
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"math"
	"net"
	"net/http"
	"strings"
)

type pair struct {
	Target string
	Route  string
	Hex    string
	Error  string
}

func ipToHex(ip net.IP, bytes int) (string, error) {
	v4 := ip.To4()
	if v4 == nil {
		return "", fmt.Errorf("only IPv4 supported: %v", ip)
	}
	if bytes < 0 || bytes > 4 {
		return "", fmt.Errorf("invalid bytes: %d", bytes)
	}
	b := v4[:bytes]
	return hex.EncodeToString(b), nil
}

func cidrPrefixBytes(prefix int) int {
	if prefix <= 0 {
		return 0
	}
	return int(math.Ceil(float64(prefix) / 8.0))
}

func buildHexString(targetCIDR string, routeIP string) (string, error) {
	ip, ipNet, err := net.ParseCIDR(targetCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid target CIDR: %w", err)
	}
	ones, _ := ipNet.Mask.Size()
	bytesNeeded := cidrPrefixBytes(ones)

	targetHex, err := ipToHex(ip, bytesNeeded)
	if err != nil {
		return "", fmt.Errorf("target ip error: %w", err)
	}

	r := net.ParseIP(routeIP)
	if r == nil {
		return "", fmt.Errorf("invalid route IP: %s", routeIP)
	}
	rHex, err := ipToHex(r, 4)
	if err != nil {
		return "", fmt.Errorf("route ip error: %w", err)
	}

	prefixHex := fmt.Sprintf("%02x", ones)
	result := "0x" + prefixHex + targetHex + rHex
	return result, nil

}

func parseHexStream(hexStr string) ([]pair, error) {
	hexStr = strings.TrimSpace(hexStr)
	if strings.HasPrefix(hexStr, "0x") || strings.HasPrefix(hexStr, "0X") {
		hexStr = hexStr[2:]
	}
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("invalid hex string: %w", err)
	}

	var results []pair
	i := 0
	for i < len(data) {
		if i+1 > len(data) {
			return results, fmt.Errorf("unexpected end of data")
		}
		prefixLen := int(data[i])
		i++
		bytesNeeded := cidrPrefixBytes(prefixLen)
		if i+bytesNeeded+4 > len(data) {
			return results, fmt.Errorf("not enough data for record")
		}

		netPart := data[i : i+bytesNeeded]
		i += bytesNeeded
		routePart := data[i : i+4]
		i += 4

		netIP := make([]byte, 4)
		copy(netIP, []byte{0, 0, 0, 0})
		copy(netIP, netPart)

		target := fmt.Sprintf("%s/%d", net.IPv4(netIP[0], netIP[1], netIP[2], netIP[3]).String(), prefixLen)
		route := net.IPv4(routePart[0], routePart[1], routePart[2], routePart[3]).String()

		fullHex := append([]byte{byte(prefixLen)}, append(netPart, routePart...)...)
		hexStr := fmt.Sprintf("%x", fullHex)
		results = append(results, pair{Target: target, Route: route, Hex: "0x" + hexStr})
	}
	return results, nil
}

var tmpl = template.Must(template.New("page").Parse(`
<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<title>HexNet конвертер</title>
<style>
 body { font-family: sans-serif; margin: 20px; }
 textarea { width: 100%; height: 150px; }
 table { border-collapse: collapse; margin-top: 20px; }
 th, td { border: 1px solid #ccc; padding: 6px 12px; }
 th { background: #eee; }
</style>
</head>
<body>
<h1>HexNet конвертер для DHCP MikroTik 121 и 249</h1>
<p>Введите по строкам либо:<br>
 - targetCIDR routeIP<br>
 - hexstream (например 0x18c0a800c0a80001, может содержать несколько записей)</p>
<form method="POST">
<textarea name="data" placeholder="192.168.0.0/24 192.168.0.1"></textarea><br>
<input type="submit" value="Convert">
</form>
{{if .}}
<table>
<tr><th>Target</th><th>Route</th><th>0хPrefixTragetRoute</th><th>Error</th></tr>
{{range .}}
<tr>
 <td>{{.Target}}</td>
 <td>{{.Route}}</td>
 <td>{{.Hex}}</td>
 <td style="color:red">{{.Error}}</td>
</tr>
{{end}}
</table>
{{end}}
</body>
</html>
`))

func pageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		tmpl.Execute(w, nil)
		return
	}
	if r.Method == http.MethodPost {
		data := r.FormValue("data")
		lines := strings.Split(strings.TrimSpace(data), "\n")
		var results []pair
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) == 1 { // hex stream
				h := parts[0]
				recs, err := parseHexStream(h)
				if err != nil {
					results = append(results, pair{Hex: h, Error: err.Error()})
				} else {
					results = append(results, recs...)
				}
			} else if len(parts) == 2 {
				t, rIP := parts[0], parts[1]
				res, err := buildHexString(t, rIP)
				p := pair{Target: t, Route: rIP}
				if err != nil {
					p.Error = err.Error()
				} else {
					p.Hex = res
				}
				results = append(results, p)
			} else {
				results = append(results, pair{Error: "line format invalid: " + line})
			}
		}
		tmpl.Execute(w, results)
		return
	}
	w.WriteHeader(http.StatusMethodNotAllowed)
}

func main() {
	http.HandleFunc("/", pageHandler)
	addr := ":8080"
	log.Printf("hexnet service with UI listening on %s\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
