import psutil
for conn in psutil.net_connections(kind="tcp"):
    if conn.laddr and conn.laddr.port == 8090 and conn.status == "LISTEN":
        print(conn.pid)
        break
