package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
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
	dstAddr := "183.59.168.27:554" // RTSP服务器的地址和端口
	channelName := r.URL.Path[6:] // 获取主机1请求的频道名，去掉/rtsp/前缀
	rtspURL, ok := channelMap[channelName] // 根据频道名查找对应的RTSP地址，如果不存在，则返回错误
	if !ok {
		http.Error(w, "Invalid channel name", http.StatusBadRequest)
		return
	}
	rtspReq, err := http.NewRequest("DESCRIBE", rtspURL, nil) // 创建一个RTSP请求，方法为DESCRIBE，用于获取媒体信息
	if err != nil {
		log.Println(err)
		return
	}
	rtspReq.Header.Set("CSeq", "1") // 设置RTSP请求头中的CSeq字段，表示请求序号为1
	rtspReq.Header.Set("User-Agent", "Apache-HttpClient/UNAVAILABLE (java 1.4)") // 设置RTSP请求头中的User-Agent字段
	rtspReq.Header.Set("Accept", "application/sdp") // 设置RTSP请求头中的Accept字段，表示接受SDP格式的媒体信息

	dstConn, err := net.Dial("tcp", dstAddr) // 创建一个到RTSP服务器的TCP连接
	if err != nil {
		log.Println(err)
		return
	}
	defer dstConn.Close()

	err = rtspReq.Write(dstConn) // 将RTSP请求写入到TCP连接中
	if err != nil {
		log.Println(err)
		return
	}

	rtspRes, err := http.ReadResponse(bufio.NewReader(dstConn), rtspReq) // 从TCP连接中读取RTSP响应
	if err != nil {
		log.Println(err)
		return
	}
	defer rtspRes.Body.Close()

	w.Header().Set("Content-Type", "application/sdp") // 设置HTTP响应头中的Content-Type字段，表示返回SDP格式的媒体信息

	io.Copy(w, rtspRes.Body) // 将RTSP响应体复制到HTTP响应体中
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
			return errors.New("Unable to split URL: " + channelURL)
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
