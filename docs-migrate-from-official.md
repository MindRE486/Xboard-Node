# 从官方版本迁移到本仓库

本仓库是 [`cedar2025/Xboard-Node`](https://github.com/cedar2025/Xboard-Node) 的长期分支：配置格式、内核、面板协议完全兼容，额外包含已审查合入的上游 PR 修复，以及可选的内置伪装站点。迁移只需要替换二进制或镜像，**不要删除已有配置**。

## 迁移前

```bash
# 记录当前版本，便于回滚
/usr/local/bin/xbctl version

# 备份配置、证书和安装元数据
sudo tar -C /etc -czf /root/xboard-node-backup-$(date +%F).tgz xboard-node
```

## 方式一：systemd / OpenRC（推荐）

安装器会识别已有安装，只替换二进制和 service 文件，保留 `config.yml`、证书与绑定信息。

```bash
curl -fsSL https://raw.githubusercontent.com/MindRE486/Xboard-Node/dev/install.sh | \
  sudo bash -s -- upgrade
```

升级完成后确认：

```bash
xbctl version
xbctl status
curl -fsS http://127.0.0.1:65530/healthz
```

> 健康检查端口以配置文件中的 `health_port` 为准；未配置时默认 65530。

## 方式二：Docker

镜像路径改为本仓库：

```bash
docker pull ghcr.io/mindre486/xboard-node:latest
```

保留原来的 `config.yml` 挂载或环境变量，替换容器即可。例如：

```bash
docker rm -f xboard-node
docker run -d --restart=always --network=host --name xboard-node \
  -v /etc/xboard-node:/etc/xboard-node \
  ghcr.io/mindre486/xboard-node:latest -c /etc/xboard-node/config.yml
```

Compose 部署只需把 `image:` 改成 `ghcr.io/mindre486/xboard-node:latest`，其余不变。

## 方式三：手动替换二进制

```bash
sudo systemctl stop xboard-node    # OpenRC: sudo rc-service xboard-node stop
sudo curl -fsSL -o /usr/local/bin/xboard-node \
  https://github.com/MindRE486/Xboard-Node/releases/latest/download/xboard-node-linux-amd64
sudo curl -fsSL -o /usr/local/bin/xbctl \
  https://github.com/MindRE486/Xboard-Node/releases/latest/download/xbctl-linux-amd64
sudo chmod 755 /usr/local/bin/xboard-node /usr/local/bin/xbctl
sudo systemctl start xboard-node   # OpenRC: sudo rc-service xboard-node start
```

ARM64 机器把 `amd64` 换成 `arm64`。

## 之后的内置更新

`xbctl upgrade` 与 `install.sh` 的下载地址已指向本仓库的 Releases，之后直接使用即可：

```bash
sudo xbctl upgrade
```

## 可选：启用伪装站点

伪装站只对 **xray + TLS + TCP + VLESS/Trojan** 节点生效，不需要面板配置，也不会额外开放公网端口。在 `config.yml` 中追加：

```yaml
camouflage:
  enabled: true
  template: 0        # 0 = 随机，1-9 指定 v2ray-agent 模板
```

然后执行 `xbctl service restart`。首次启动会下载模板；下载失败时会写入内置页面并在日志中告警。

## 回滚到官方版本

```bash
sudo systemctl stop xboard-node
# 用官方安装器覆盖二进制即可，配置不需要变动
curl -fsSL https://raw.githubusercontent.com/cedar2025/xboard-node/dev/install.sh | \
  sudo bash -s -- upgrade
```

如需完全恢复：

```bash
sudo tar -C /etc -xzf /root/xboard-node-backup-<日期>.tgz
```

## 注意事项

- 两个版本的 `config.yml` 双向兼容；迁移不会重写现有配置。
- 本仓库的 `xbctl version` 使用自己的版本号（例如 `v1.14`），与官方版本号不连续，属于正常现象。
- 如果同时混用官方与本仓库的二进制，请保持 `xboard-node` 与 `xbctl` 来自同一个 release。
