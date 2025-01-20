package main

import (
	"fmt"
	"log"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

func main() {

	cpuInfo, err := cpu.Info()
	if err != nil {
		log.Fatalf("Error fetching CPU info: %v", err)
	}
	fmt.Println("=== CPU Info ===")
	for _, cpu := range cpuInfo {
		fmt.Printf("Model: %s\nCores: %d\nSpeed: %.2f GHz\n\n",
			cpu.ModelName, cpu.Cores, cpu.Mhz/1000)
	}

	memInfo, err := mem.VirtualMemory()
	if err != nil {
		log.Fatalf("Error fetching memory info: %v", err)
	}
	fmt.Println("=== Memory Info ===")
	fmt.Printf("Total Memory: %.2f GB\nUsed Memory: %.2f GB\nFree Memory: %.2f GB\n\n",
		float64(memInfo.Total)/1e9, float64(memInfo.Used)/1e9, float64(memInfo.Free)/1e9)

	diskInfo, err := disk.Usage("/")
	if err != nil {
		log.Fatalf("Error fetching disk info: %v", err)
	}
	fmt.Println("=== Disk Info ===")
	fmt.Printf("Total Disk Space: %.2f GB\nUsed Disk Space: %.2f GB\nFree Disk Space: %.2f GB\n\n",
		float64(diskInfo.Total)/1e9, float64(diskInfo.Used)/1e9, float64(diskInfo.Free)/1e9)

	hostInfo, err := host.Info()
	if err != nil {
		log.Fatalf("Error fetching host info: %v", err)
	}
	fmt.Println("=== Host Info ===")
	fmt.Printf("Hostname: %s\nOS: %s %s\nUptime: %d seconds\n\n",
		hostInfo.Hostname, hostInfo.Platform, hostInfo.PlatformVersion, hostInfo.Uptime)
}
