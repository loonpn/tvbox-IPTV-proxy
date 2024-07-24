# tvbox-IPTV-proxy
安卓机顶盒通过光猫IPTV拨号，读取机顶盒直播源组播地址并在局域网内代理，实现一号多终端。
## 使用方法
1.使用本项目提供的 Github Action 编译。
2.将编译后的文件放到机顶盒 /data/local 目录下，chmod +x 添加可执行权限。
3.在 /system/etc/install-recovery.sh 文件中添加 "/data/local/myapp"。
