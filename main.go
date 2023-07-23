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
	path = flag.String("path", "/data/data/com.huawei.channellist.contentprovider/databases/channelURL.db", "path to the database file")
	sqlfile  = flag.String("sqlfile", "/data/local/output.sql", "SQL file to execute")
	channelList map[string]string
	addr = flag.String("l", ":18000", "Listening address")
	iface = flag.String("i", "eth0", "Listening multicast interface")
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
	return channelList[id]
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
		return
	}

	// 检查id参数是否存在
	id := req.FormValue("id")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "Missing id parameter")
		return
	}

	if !existId(id) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "Channel ID not found")
		return
	}

	// 解析URL
	re := regexp.MustCompile(`(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}):(\d{1,5})`)
	match := re.FindStringSubmatch(get(id))
	if match == nil {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "Error when parsing url:" + get(id))
		return
	}

	// 获取主机和端口
	raddr := match[0]

	addr, err := net.ResolveUDPAddr("udp4", raddr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, err.Error())
		return
	}

	conn, err := net.ListenMulticastUDP("udp4", inf, addr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, err.Error())
		return
	}
	defer conn.Close()

	// 设置连接超时时间
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)

	// 复制数据到响应写入器，并检查错误
	n, err := io.Copy(w, conn)
	if err != nil {
		log.Printf("Error when copying data: %v\n", err)
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