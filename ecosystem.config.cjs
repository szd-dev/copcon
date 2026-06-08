// PM2 配置文件 - CopCon 前后端服务管理
const path = require('path');

module.exports = {
  apps: [
    // ─── 后端：Go API Server ──────────────────────────────
    {
      name: 'copcon-server',
      script: 'go',
      args: 'run server/cmd/server/main.go',
      interpreter: 'none',                     // 直接执行 go 二进制，不用 node 包装
      cwd: __dirname,                          // 项目根目录，确保 skills 发现 .copcon/skills
      env: {
        CONFIG_PATH: path.join(__dirname, 'server/config.yaml'),
      },
      autorestart: true,
      watch: false,
      restart_delay: 2000,
      max_restarts: 5,
      min_uptime: '10s',
      kill_timeout: 5000,
      merge_logs: true,
      log_date_format: 'YYYY-MM-DD HH:mm:ss Z',
    },

    // ─── 前端：Vite Dev Server ─────────────────────────────
    {
      name: 'copcon-demo',
      script: 'pnpm',
      args: 'dev',
      interpreter: 'none',                     // 直接执行 pnpm 二进制
      cwd: path.join(__dirname, 'packages/demo'),
      autorestart: false,                      // Vite 自带 HMR，不需要 PM2 重启
      watch: false,
      kill_timeout: 3000,
      merge_logs: true,
      log_date_format: 'YYYY-MM-DD HH:mm:ss Z',
    },
  ],
};