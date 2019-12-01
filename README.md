# goproxy  
用于TCP流量的内网穿透　　
- 支持心跳检测
- 支持断线重连
- 支持自定义端口
## client.go  
部署在内网服务器上　　　　
示例：　　
`go run client.go -host 234.234.234.234 -localPort 8080 -remotePort 20012`  
## server.go  
部署在公网服务器上　　　　　
示例：　　
`go run server.go -localPort 3002 -remotePort 20012`    
##　结果　　
![](http://pyw7z52y6.bkt.clouddn.com/Fh641WP9ccjV2H7slWoMLdaDjE0b)
