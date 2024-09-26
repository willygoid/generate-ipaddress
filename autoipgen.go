package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

const (
	maxOctet   = 255
	dirName    = "result"
	batchSize  = 10000000 // Meningkatkan ukuran batch untuk meminimalkan operasi IO
)

// Daftar Bogon IP (IP yang tidak digunakan untuk publik di internet)
var bogonRanges = []string{
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"192.168.0.0/16",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"224.0.0.0/4",
	"240.0.0.0/4",
}

// Bogon ranges in integer format for faster comparison
var bogonIntRanges = []struct {
	start, end uint32
}{}

// Fungsi utama
func main() {
	// Memanfaatkan semua core yang tersedia
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Konversi Bogon ranges ke dalam format integer untuk mempercepat validasi
	for _, cidr := range bogonRanges {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		startIP := ipToUint32(ipNet.IP)
		broadcastIP := ipToUint32(getBroadcastAddress(ipNet))
		bogonIntRanges = append(bogonIntRanges, struct {
			start, end uint32
		}{startIP, broadcastIP})
	}

	// Memilih opsi
	fmt.Println("Pilih opsi:")
	fmt.Println("1. Generate IP berdasarkan range")
	fmt.Println("2. Generate IP berdasarkan prefix")
	fmt.Println("3. Generate semua IP")
	var option int
	fmt.Scan(&option)

	// Membuat folder result jika belum ada
	if err := os.MkdirAll(dirName, 0755); err != nil {
		fmt.Println("Gagal membuat folder result:", err)
		return
	}

	switch option {
	case 1:
		handleRangeOption()
	case 2:
		handlePrefixOption()
	case 3:
		handleAllIPOption()
	default:
		fmt.Println("Pilihan tidak valid.")
	}
}

// Opsi 1: Generate IP berdasarkan range
func handleRangeOption() {
	var startIP, endIP string
	fmt.Print("Masukkan IP awal: ")
	fmt.Scan(&startIP)
	fmt.Print("Masukkan IP akhir: ")
	fmt.Scan(&endIP)

	start := net.ParseIP(startIP)
	end := net.ParseIP(endIP)

	if start == nil || end == nil {
		fmt.Println("IP tidak valid.")
		return
	}

	fmt.Println("Mulai menghasilkan IP dari", startIP, "ke", endIP)
	generateRangeIPs(start, end)
}

// Opsi 2: Generate IP berdasarkan prefix
func handlePrefixOption() {
	var ipPrefix string
	fmt.Print("Masukkan IP dan prefix (misal 192.168.0.0/16): ")
	fmt.Scan(&ipPrefix)

	_, ipNet, err := net.ParseCIDR(ipPrefix)
	if err != nil {
		fmt.Println("Prefix tidak valid.")
		return
	}

	fmt.Println("Mulai menghasilkan IP untuk prefix", ipPrefix)
	generatePrefixIPs(ipNet)
}

// Opsi 3: Generate semua IP
func handleAllIPOption() {
	fmt.Println("Generate semua IP atau tanpa Bogon?")
	fmt.Println("1. Semua IP")
	fmt.Println("2. Tanpa Bogon")
	var choice int
	fmt.Scan(&choice)

	if choice == 1 {
		fmt.Println("Mulai menghasilkan semua IP...")
		generateAllIPs(false)
	} else if choice == 2 {
		fmt.Println("Mulai menghasilkan semua IP tanpa Bogon...")
		generateAllIPs(true)
	} else {
		fmt.Println("Pilihan tidak valid.")
	}
}

// Fungsi untuk menghasilkan rentang IP
func generateRangeIPs(start, end net.IP) {
	var wg sync.WaitGroup
	ipChannel := make(chan []string, runtime.NumCPU()*2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		writeToFile(ipChannel)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		generateIPsInRange(start, end, ipChannel)
	}()

	wg.Wait()
	close(ipChannel)
}

// Fungsi untuk menghasilkan IP dari prefix
func generatePrefixIPs(ipNet *net.IPNet) {
	var wg sync.WaitGroup
	ipChannel := make(chan []string, runtime.NumCPU()*2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		writeToFile(ipChannel)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		generateIPsFromPrefix(ipNet, ipChannel)
	}()

	wg.Wait()
	close(ipChannel)
}

// Fungsi untuk menghasilkan semua IP atau tanpa Bogon
func generateAllIPs(skipBogon bool) {
	var wg sync.WaitGroup
	ipChannel := make(chan []string, runtime.NumCPU()*2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		writeToFile(ipChannel)
	}()

	for firstOctet := 0; firstOctet <= maxOctet; firstOctet++ {
		wg.Add(1)
		go func(firstOctet int) {
			defer wg.Done()
			generateIPs(firstOctet, ipChannel, skipBogon)
		}(firstOctet)
	}

	wg.Wait()
	close(ipChannel)
}

// Fungsi untuk menghasilkan IP dalam rentang
func generateIPsInRange(start, end net.IP, ipChannel chan<- []string) {
	startInt := ipToUint32(start)
	endInt := ipToUint32(end)

	for current := startInt; current <= endInt; current++ {
		ip := uint32ToIP(current)
		ipChannel <- []string{ip.String() + "\n"}
	}
}

// Fungsi untuk menghasilkan IP dari prefix
func generateIPsFromPrefix(ipNet *net.IPNet, ipChannel chan<- []string) {
	start := ipNet.IP
	end := getBroadcastAddress(ipNet)

	generateIPsInRange(start, end, ipChannel)
}

// Fungsi untuk mengubah IP ke bilangan 32-bit
func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

// Fungsi untuk mengubah bilangan 32-bit ke IP
func uint32ToIP(n uint32) net.IP {
	return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
}

// Fungsi untuk mendapatkan alamat broadcast dari IP dan prefix
func getBroadcastAddress(ipNet *net.IPNet) net.IP {
	ip := ipNet.IP
	mask := ipNet.Mask
	broadcast := make(net.IP, len(ip))
	for i := range ip {
		broadcast[i] = ip[i] | ^mask[i]
	}
	return broadcast
}

// Fungsi untuk menghasilkan IP (semua IP atau tanpa Bogon)
func generateIPs(firstOctet int, ipChannel chan<- []string, skipBogon bool) {
	batch := make([]string, 0, batchSize)

	for secondOctet := 0; secondOctet <= maxOctet; secondOctet++ {
		for thirdOctet := 0; thirdOctet <= maxOctet; thirdOctet++ {
			for fourthOctet := 0; fourthOctet <= maxOctet; fourthOctet++ {
				ip := fmt.Sprintf("%d.%d.%d.%d\n", firstOctet, secondOctet, thirdOctet, fourthOctet)

				if skipBogon && isBogon(ip) {
					continue
				}

				batch = append(batch, ip)

				if len(batch) >= batchSize {
					ipChannel <- batch
					batch = make([]string, 0, batchSize)
				}
			}
		}
	}

	if len(batch) > 0 {
		ipChannel <- batch
	}
}

// Fungsi untuk memeriksa apakah IP ada di dalam Bogon range menggunakan integer comparison
func isBogon(ip string) bool {
	ipInt := ipToUint32(net.ParseIP(ip))
	for _, r := range bogonIntRanges {
		if ipInt >= r.start && ipInt <= r.end {
			return true
		}
	}
	return false
}

// Fungsi untuk menulis ke file
func writeToFile(ipChannel <-chan []string) {
	for batch := range ipChannel {
		if len(batch) == 0 {
			continue
		}

		firstIP := strings.Split(batch[0], ".")
		firstOctet, _ := strconv.Atoi(firstIP[0])
		fileName := fmt.Sprintf("%d.txt", firstOctet)
		filePath := filepath.Join(dirName, fileName)

		file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Printf("Gagal membuka file %s: %v\n", filePath, err)
			continue
		}

		writer := bufio.NewWriter(file)
		for _, ip := range batch {
			_, _ = writer.WriteString(ip)
		}
		writer.Flush()
		file.Close()
	}
}
