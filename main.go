package main

import (
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// 定义一个Channel结构体，用于存储频道信息
type Channel struct {
	UserChannelID string `json:"UserChannelID"`
	ChannelNo     string `json:"ChannelNo"`
	ChannelName   string `json:"ChannelName"`
	ChannelURL    string `json:"ChannelURL"`
	PreviewURL    string `json:"PreviewURL"`
	Ext           string `json:"ext"`
}

var (
	dbpath = flag.String("dbpath", "/data/data/com.huawei.channellist.contentprovider/databases/channelURL.db", "Path to the database file")
	port = flag.String("port", ":8080", "Listening port")
	sqlpath  = flag.String("sqlpath", "/data/local/output.sql", "Path to the sql file")
	channelMap map[string]string // 定义一个全局变量，用于存储频道名和RTSP地址的映射关系
)

// 定义一个HTTP处理器函数，用于将HTTP请求转换为RTSP请求，并发送到目标地址
func rtspHandler(w http.ResponseWriter, r *http.Request) {
	channelName := r.URL.Path[6:] // 获取主机1请求的频道名，去掉/rtsp/前缀
	rtspURL, ok := channelMap[strings.Replace(channelName, " ", "", -1)] // 根据频道名查找对应的RTSP地址，如果不存在，则返回错误
	if !ok {
		http.Error(w, "Invalid channel name", http.StatusBadRequest)
		return
	}

	var dstConn net.Conn // 声明一个TCP连接变量
	//var err error // 声明一个错误变量

	for { // 使用一个循环，直到找到最终的RTSP地址
		// 解析URL
		u, err := url.Parse(rtspURL)
		if err != nil {
			http.Error(w, "Error when parsing url: " + rtspURL, http.StatusBadRequest)
			log.Printf("Error when parsing url: %s\n", rtspURL)
			return
		}
		//dstConn, err = net.Dial("udp", strings.Replace(u.Host, ":554", "", -1) + ":554") // 创建一个udp连接，连接到RTSP服务器
		dstConn, err = net.Dial("tcp", strings.Replace(u.Host, ":554", "", -1) + ":554") // 创建一个udp连接，连接到RTSP服务器
		if err != nil {
			log.Println(err)
			return
		}
		defer dstConn.Close()

		//rtspReq := "DESCRIBE " + rtspURL + " RTSP/1.0\r\nCSeq: 1\r\nUser-Agent: Go-RTSP-Client\r\nAccept: application/sdp\r\nTransport:RTP/AVP;unicast\r\n\r\n" // 构造一个RTSP DESCRIBE请求
		rtspReq := "DESCRIBE " + strings.Replace(rtspURL, ":554", "", -1) + " RTSP/1.0\r\nCSeq: 1\r\nUser-Agent: Go-RTSP-Client\r\nAccept: application/sdp\r\n\r\n" // 构造一个RTSP DESCRIBE请求
		_, err = dstConn.Write([]byte(rtspReq)) // 将RTSP请求发送到UDP连接中
		if err != nil {
			log.Println(err)
			return
		}

		buf := make([]byte, 2048) // 创建一个缓冲区，用于存储从UDP连接中读取的数据
		n,err := dstConn.Read(buf) // 从TCP连接中读取数据，可能包含RTSP响应和SDP信息
		//n, _, err := dstConn.(*net.UDPConn).ReadFrom(buf) // 从TCP连接中读取数据，可能包含RTSP响应和SDP信息
		if err != nil {
			log.Println(err)
			return
		}

		if strings.HasPrefix(string(buf[:n]), "RTSP/1.0 302") { // 检查是否收到了重定向响应
			location := regexp.MustCompile(`Location: (.*)\r\n`).FindStringSubmatch(string(buf[:n])) // 从响应中提取Location字段的值
			if len(location) > 1 {
				rtspURL = location[1] // 更新新的RTSP地址
				log.Println("RTSP/1.0 302" + rtspURL)
				continue // 继续循环，直到找到最终的RTSP地址
			} else {
				http.Error(w, "Invalid Location header", http.StatusInternalServerError)
				return
			}
		}
		if strings.HasPrefix(string(buf[:n]), "RTSP/1.0 403") {
			http.Error(w, "Remote server response: 403 Forbidden", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/sdp") // 设置HTTP响应头中的Content-Type字段，表示返回SDP格式的媒体信息

		w.Write(buf[:n]) // 将从TCP连接中读取的数据写入到HTTP响应体中

		break // 跳出循环，结束函数
	}
}

// 定义一个函数，使用shell命令，从sqlite数据库文件中读取json数据，并保存到全局变量channelMap中
func readFile(dbpath, sqlpath string) error {
	
	sqlFile, err := os.Open(sqlpath)
	if err != nil {
		return err
	}
	defer sqlFile.Close()

	cmd := exec.Command("sqlite3", dbpath)

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

	for _, channel := range channels { // 遍历切片中的每个频道
		parts := strings.Split(channel.ChannelURL, "|") // 用 | 分割字符串
		if len(parts) != 2 {
			// 处理错误情况
			return errors.New("Unable to split string with \"|\": " + channel.ChannelURL)
		}
		channelMap[strings.Replace(channel.ChannelName," ", "", -1)] = parts[1] // 将频道名和RTSP地址存储到映射关系中
	}
	return nil
}

func main() {
	flag.Parse()
	channelMap = make(map[string]string) // 初始化频道映射关系
	err := readFile(*dbpath, *sqlpath) // 读取数据库数据写入channelMap
	if err != nil {
		log.Fatal(err)
		return
	}
	http.HandleFunc("/rtsp/", rtspHandler) // 注册一个HTTP处理器函数，用于处理/rtsp/路径的请求
	log.Printf("Listening port %s\n", *port)
	log.Fatal(http.ListenAndServe(*port, nil)) // 监听本地8080端口，并启动HTTP服务器
}
