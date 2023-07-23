package main

import (
  "flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
  "regexp"
	"strings"
  "database/sql"
  _ "github.com/mattn/go-sqlite3"
)

var (
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


func readFile(path, cmd string) error {
    db, err := sql.Open("sqlite3", path)
  if err != nil {
    return err
  }
  defer db.Close()
  rows, err := db.Query(cmd)
  if err != nil {
    return err
  }
  defer rows.Close()
  for rows.Next() {
    var data string
    err = rows.Scan(&data)
    if err != nil {
      return err
    }
    // 解析JSON数组
    var channels []Channel // Channel是定义的结构体类型
    err = json.Unmarshal([]byte(data), &channels)
    if err != nil {
      return err
    }
    // 输出结果
    for _, c := range channels {
      add(strings.TrimSpace(c.ChannelName), c.ChannelURL)
    }
  }
  err = rows.Err()
  if err != nil {
    return err
  }
  return nil
}

func handleHTTP(w http.ResponseWriter, req *http.Request) {
  req.ParseForm()
	// 获取URL参数
	id := r.FormValue("id")
	if len(id) < 2 {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "No id specified")
		return
	}
  if !hasId(id) {
    w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "Channel ID not found")
		return
  }
  pattern := regexp.MustCompile("(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}):(\d{1,5})")
  match := re.FindStringSubmatch(get(id))
  if match == nil {
    w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "Error when parsing url:" + get(id))
		return
  }
	raddr := match[1] + ":" + match[2]
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

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	n, err := io.Copy(w, conn)
	log.Printf("%s %s %d [%s]", req.RemoteAddr, req.URL.Path, n, req.UserAgent())
}

func main(){
  if os.Getppid() == 1 {
		log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))
	} else {
		log.SetFlags(log.Lshortfile | log.LstdFlags)
	}
	flag.Parse()
  
  err := readFile("/data/data/com.huawei.channellist.contentprovider/databases/channelURL.db", "SELECT livechannels FROM channleList")
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
