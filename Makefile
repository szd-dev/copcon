.PHONY: dev restart restart-server restart-demo logs status stop clean

# 一键启动所有服务
dev:
	pm2 start ecosystem.config.cjs
	pm2 logs

# 重启所有服务
restart:
	pm2 restart ecosystem.config.cjs --update-env
	pm2 logs

# 只重启后端
restart-server:
	pm2 restart copcon-server
	pm2 logs copcon-server

# 只重启前端
restart-demo:
	pm2 restart copcon-demo
	pm2 logs copcon-demo

# 查看实时日志
logs:
	pm2 logs

# 查看运行状态
status:
	pm2 list

# 停止所有服务
stop:
	pm2 stop all

# 停止并删除进程（彻底清理）
clean:
	pm2 delete all