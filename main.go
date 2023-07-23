package main

import (
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"os"
	"os/exec"
	"strings"
	"encoding/json"
	"time"
)

var (
	path = flag.String("path", "/data/data/com.huawei.channellist.contentprovider/databases/channelURL.db", "Path to the database file")
	addr = flag.String("l", ":18000", "Listening address")
	iface = flag.String("i", "eth0", "Listening multicast interface")
	sqlfile  = flag.String("sqlfile", "/data/local/output.sql", "SQL file to execute")
	channelList map[string]string
	inf *net.Interface
)

type Channel struct {
	UserChannelID string `json:"UserChannelID"`
	ChannelNo     string `json:"ChannelNo"`
	ChannelName   string `json:"ChannelName"`
	ChannelURL    string `json:"ChannelURL"`
	PreviewURL    string `json:"PreviewURL"`
	Ext           string `json:"ext"`
}

func add(id string, url1 string) {
	channelList[id] = url1
}

func get(id string) string {
	// 假设 channelList 是一个 map 类型的变量，存储多播地址和单播地址的对应关系
	channelURL := channelList[id] // 获取多播地址和单播地址的组合字符串
	parts := strings.Split(channelURL, "|") // 用 | 分割字符串
	if len(parts) != 2 {
		// 处理错误情况
		log.Printf("get: invalid channel URL: %s", channelURL)
		return ""
	}
	return parts[1] // 返回单播地址
}


func existId(id string) bool {
	_, ok := channelList[id]
	return ok
}


func readFile(path, sqlfile string) error {	
	sqlFile, err := os.Open(sqlfile)
	if err != nil {
		return err
	}
	defer sqlFile.Close()

	cmd := exec.Command("sqlite3", path)

	cmd.Stdin = sqlFile

	output, err := cmd.Output()
	if err != nil {
		return err
	}

	var channels []Channel 
	err = json.Unmarshal([]byte(output), &channels)
	if err != nil {
		return err
	}

	for _, c := range channels {
		add(strings.Replace(c.ChannelName," ", "", -1), c.ChannelURL)
	}
	return nil
}

func handleHTTP(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()

	// 检查请求方法是否是GET
	if req.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		io.WriteString(w, "Only GET method is allowed")
		log.Println("Only GET method is allowed")
		return
	}

	// 检查id参数是否存在
	id := req.FormValue("id")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "Missing id parameter: id")
		log.Println("Missing id parameter: id")
		return
	}

	if !existId(id) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "Channel ID not found")
		log.Printf("Channel ID not found: %s\n", id)
		return
	}

	// 解析URL
	re := regexp.MustCompile(`(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}):(\d{1,5})`)
	match := re.FindStringSubmatch(get(id))
	if match == nil {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "Error when parsing url:" + get(id))
		log.Printf("Error when parsing url: %s\n", get(id))
		return
	}

	// 获取主机和端口
	raddr := match[0]

	addr, err := net.ResolveUDPAddr("udp4", raddr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, err.Error())
		log.Printf("%v\n", err)
		return
	}

	conn, err := net.ListenMulticastUDP("udp4", inf, addr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, err.Error())
		log.Printf("%v\n", err)
		return
	}
	defer conn.Close()
	
	// 建立一个单播连接，使用 map 中存储的单播地址
	unicastAddr := get(id) // 假设这个函数可以从 map 中获取单播地址
	uconn, err := net.Dial("udp4", unicastAddr)
	if err != nil {
 		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, err.Error())
		return
	}
	defer uconn.Close()
	
	// 设置连接超时时间
	//conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)

	// 将数据从多播连接复制到单播连接
	go func() {
		_, err := io.Copy(uconn, conn)
		if err != nil {
			// 处理复制错误
			log.Printf("handleHTTP: io.Copy error: %v", err)
			return
		}
	}()

	// 复制数据到响应写入器，并检查错误
	n, err := io.Copy(w, conn)
	if err != nil {
		log.Printf("handleHTTP: io.Copy error: %v, raddr = %s, addr =  %s\n", err, raddr, addr)
		return
	}
	log.Printf("%s %s %d [%s]", req.RemoteAddr, req.URL.Path, n, req.UserAgent())
}

func main(){
	if os.Getppid() == 1 {
		log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))
	} else {
		log.SetFlags(log.Lshortfile | log.LstdFlags)
	}
	flag.Parse()
	if *path == "" || *sqlfile == "" {
		log.Println("Missing path or sqlfile parameters. Use default values.")
	}
	channelList = make(map[string]string)
	err := readFile(*path, *sqlfile)
	if err != nil {
		log.Fatal(err)
	}

	inf, err = net.InterfaceByName(*iface)
	if err != nil {
		log.Fatal(err)
		return
	}

	var mux http.ServeMux
	mux.HandleFunc("/rtp", handleHTTP)

	log.Fatal(http.ListenAndServe(*addr, &mux))
}
