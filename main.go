package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/getlantern/systray"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"golang.org/x/sys/windows/registry"

	"syscat/resource/icon"
)


type syscatConfig struct {
	Theme       string
	IsAutostart bool
}

var catConfig = syscatConfig{
	Theme: "blackcat",
	IsAutostart: false,
}
var configFile string



func main() {
	onExit := func() {
		now := time.Now()
		ioutil.WriteFile(fmt.Sprintf(`on_exit_%d.txt`, now.UnixNano()), []byte(now.String()), 0644)
	}

	systray.Run(onReady, onExit)
}


func onReady() {
	// set Icon
	systray.SetIcon(icon.Data[0])
	systray.SetTitle("System Monitor Cat")

	// set Menu
	mCPU := systray.AddMenuItem("CPU", "Show cpu percent value")
	mMemory := systray.AddMenuItem("Mem", "Show memory percent value")
	mNetRateUp := systray.AddMenuItem("Upload", "Show up network rate")
	mNetRateDown := systray.AddMenuItem("Download", "Show down network rate")
	
	systray.AddSeparator()

	mThemeTop := systray.AddMenuItem("Theme", "Select the cat theme")
	mThemeWhite := mThemeTop.AddSubMenuItemCheckbox("White", "Select the white cat", false)
	mThemeBlack := mThemeTop.AddSubMenuItemCheckbox("Black", "Select the black cat", true)
	mClickEntryTop := systray.AddMenuItem("Entry", "Select the click entry")
	mClickEntry1 := mClickEntryTop.AddSubMenuItem("Tasklist", "")
	mClickEntry2 := mClickEntryTop.AddSubMenuItem("Explorer", "")
	
	systray.AddSeparator()

	mAutoStart := systray.AddMenuItemCheckbox("Autostart", "Open at boot", false)
	mQuit := systray.AddMenuItem("Quit", "Quit the whole app")

	// Monitor the quit menu
	quitMonitor := func() {
		<-mQuit.ClickedCh
		systray.Quit()
	}
	go quitMonitor()

	// define Machine Load signal
	machineload := make(chan int64, 5)

	// value and show init
	readConfig()
	runInit := func() {
		switch catConfig.Theme {
			case "blackcat":
				icon.Data = icon.Data_b
			case "whitecat":
				icon.Data = icon.Data_w
		}
		if catConfig.IsAutostart {
			mAutoStart.Check()
		}

		netInfo, _ := net.IOCounters(false)
		netUpBytes := netInfo[0].BytesSent
		netDownBytes := netInfo[0].BytesRecv
		time.Sleep(time.Duration(100) * time.Millisecond)
		netInfo, _ = net.IOCounters(false)
		netUpValue := rateToString((netInfo[0].BytesSent - netUpBytes) * 10.0)
		netDownValue := rateToString((netInfo[0].BytesRecv - netDownBytes) * 10.0)

		cpuInfo, _ := cpu.Percent(time.Second, false)
		cpuValue := percentToString(cpuInfo[0])
		memInfo, _ := mem.VirtualMemory()
		memValue := percentToString(memInfo.UsedPercent)

		systray.SetTooltip("CPU: " + cpuValue + "\n" + "Mem: " + memValue + "\n")
		mCPU.SetTitle("CPU: " + cpuValue)
		mMemory.SetTitle("Mem: " + memValue)
		mNetRateUp.SetTitle("↑: " + netUpValue)
		mNetRateDown.SetTitle("↓: " + netDownValue)

		machineload <- int64(math.Ceil(cpuInfo[0]))
	}
	runInit()

	// compute machineload and show menutext
	osInfomation := func(ch chan<- int64) {
		for {
			netInfo, _ := net.IOCounters(false)
			netUpBytes := netInfo[0].BytesSent
			netDownBytes := netInfo[0].BytesRecv

			time.Sleep(time.Duration(1) * time.Second)

			netInfo, _ = net.IOCounters(false)
			netUpValue := rateToString(netInfo[0].BytesSent - netUpBytes)
			netDownValue := rateToString(netInfo[0].BytesRecv - netDownBytes)

			cpuInfo, _ := cpu.Percent(time.Second, false)
			cpuValue := percentToString(cpuInfo[0])
			memInfo, _ := mem.VirtualMemory()
			memValue := percentToString(memInfo.UsedPercent)

			systray.SetTooltip("CPU: " + cpuValue + "\n" + "Mem: " + memValue + "\n")
			mCPU.SetTitle("CPU: " + cpuValue)
			mMemory.SetTitle("Mem: " + memValue)
			mNetRateUp.SetTitle("↑: " + netUpValue)
			mNetRateDown.SetTitle("↓: " + netDownValue)

			if len(ch) < 5 {
				ch <- int64(math.Ceil(memInfo.UsedPercent))
			}
		}
	}
	go osInfomation(machineload)

	// run the cat image
	/*
		runImage rate rules
		fast 25 >= changerate >= 225 slow
		machineload 100 - 0
		y = 225 - 2*x
		25 极限
		50 快
		75 稍快
		100 流畅快
		125 流畅
		150 稍卡流畅
		175 帧率
		200 卡顿明显
		225 卡
	*/
	runImage := func(ch <-chan int64) {
		var imageRate int64 = 125
		nowFlag := 0
		for {
			if nowFlag >= 5 {
				nowFlag = 0
				if len(ch) != 0 {
					imageRate = 225 - (2 * <-ch)
				}
			} else {
				fmt.Println(nowFlag)
				fmt.Println(imageRate)
				systray.SetIcon(icon.Data[nowFlag])
				nowFlag++
				time.Sleep(time.Duration(imageRate) * time.Millisecond)
			}
		}
	}
	go runImage(machineload)

	// monitor the other menu
	go func() {
		for {
			select {
			case <- mQuit.ClickedCh:
				systray.Quit()
				return
			case <- mAutoStart.ClickedCh:
				if mAutoStart.Checked() {
					catConfig.IsAutostart = false
					mAutoStart.Uncheck()
					go uninstallAutoStart()
					go writeConfig()
				} else {
					catConfig.IsAutostart = true
					mAutoStart.Check()
					go installAutoStart()
					go writeConfig()
				}
			case <- mClickEntry1.ClickedCh:
				go entry1()
			case <- mClickEntry2.ClickedCh:
				go entry2()
			case <- mThemeWhite.ClickedCh:
				mThemeWhite.Check()
				go whiteTheme()
				mThemeBlack.Uncheck()
				catConfig.Theme = "whitecat"
				go writeConfig()
			case <- mThemeBlack.ClickedCh:
				mThemeBlack.Check()
				go blackTheme()
				mThemeWhite.Uncheck()
				catConfig.Theme = "blackcat"
				go writeConfig()
			}
		}
	}()
}


// entry1 : open tasklist manager
func entry1() {
	exec.Command("taskmgr").Run()
}
// entry2 : open file explorer
func entry2() {
	exec.Command("explorer").Run()
}


// change cat color
func whiteTheme() {
	icon.Data = icon.Data_w
}
func blackTheme() {
	icon.Data = icon.Data_b
}

            
// network rate to String
func rateToString(ratenumber uint64) string {
	temp := float64(ratenumber) / float64(1024)
	if temp-1024.0 >= 0.0 {
		temp = temp / float64(1024)
		if temp-1024.0 >= 0.0 {
			temp = temp / float64(1024)
			return strconv.FormatFloat(temp, 'f', 2, 64) + " GB"
		} else {
			return strconv.FormatFloat(temp, 'f', 2, 64) + " MB"
		}
	} else {
		return strconv.FormatFloat(temp, 'f', 2, 64) + " KB"
	}
}

// machine load rate to String
func percentToString(percentnumber float64) string {
	return strconv.FormatFloat(percentnumber, 'f', 1, 64) + " %"
}

func readConfig() bool {
	tempUser, err := user.Current()
	switch runtime.GOOS {
		case "windows":
			if err == nil {
				configFile = tempUser.HomeDir + "\\Documents\\Syscat\\syscat.conf"
			} else {
				configFile = "./syscat.conf"
			}
		case "linux", "darwin":
			configFile =  tempUser.HomeDir + "/Syscat/syscat.conf"
		default:
			configFile = "./syscat.conf"
	}
	_ , err = os.Stat(filepath.Dir(configFile))
	if os.IsNotExist(err) {
		fmt.Println(err)
		err = os.Mkdir(filepath.Dir(configFile), 0777)
	} else if err != nil {
		fmt.Println(err)
		return false
	}

	_, err = os.Stat(configFile)
	if os.IsNotExist(err) {
		os.Create(configFile)
		os.Chmod(configFile, 0666)
		writeConfig()
	} else if err != nil {
		fmt.Println(err)
		return false
	}

	f, err := ioutil.ReadFile(configFile)
	if err != nil {
		fmt.Println(err)
		return false
	}

	err = json.Unmarshal([]byte(f), &catConfig)
	if err != nil {
		fmt.Println(err)
		return false
	} else {
		return true
	}
}

func writeConfig() bool {
	var f []byte
	f, err := json.Marshal(&catConfig)
	if err != nil {
		fmt.Println(err)
		return false
	} else {
		err := ioutil.WriteFile(configFile, f, 0666)
		if err != nil {
			fmt.Println(err)
			return false
		} else {
			return true
		}
	}
}


// set syscat autostart on boot
func installAutoStart() bool {
	currentFile, _ := os.Executable()
	_, currentProgram := filepath.Split(currentFile)

	_, err := os.Stat(filepath.Dir(configFile))
	if os.IsNotExist(err) {
		os.Mkdir(filepath.Dir(configFile), 0777)
	} else if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Dir(configFile) + "\\" + currentProgram)
	if os.IsNotExist(err) {
		exec.Command("cmd", "/c", "copy", "/b", currentFile, filepath.Dir(configFile)+"\\"+currentProgram).Run()
	} else if err != nil {
		return false
	}

	autoStartKey, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Run`, registry.ALL_ACCESS)

	if err != nil {
		fmt.Println(err)
		return false
	}

	autoValue, _, err := autoStartKey.GetStringValue("Syscat")
	if err != nil {
		fmt.Println(err)
		err = autoStartKey.SetStringValue(`Syscat`, filepath.Dir(configFile)+"\\"+currentProgram)
		if err != nil {
			fmt.Println(err)
		}
	} else {
		fmt.Printf("syscat value : %q\n", autoValue)
	}

	defer autoStartKey.Close()

	return true
}

func uninstallAutoStart() bool {
	autoStartKey, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Run`, registry.ALL_ACCESS)

	if err != nil {
		fmt.Println(err)
		return false
	}

	autoValue, _, err := autoStartKey.GetStringValue("Syscat")
	if err != nil {
		fmt.Println(2, err)
		return false
	} else {
		fmt.Printf("syscat value : %q\n", autoValue)
		autoStartKey.DeleteValue(`Syscat`)
	}

	defer autoStartKey.Close()

	return true
}

