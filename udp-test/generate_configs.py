#!/usr/bin/env python3
"""
生成客户端配置文件的独立脚本
"""

import yaml
from pathlib import Path

CONFIG_FILE = Path(__file__).parent / "config.yaml"

def generate_client_config(folder, client_id, secret_key, server_addr, server_port):
    """生成客户端配置文件"""
    config_content = f"""# Tunnox Client Configuration
client_id: {client_id}
auth_token: "{secret_key}"
anonymous: false

server:
  protocol: udp
  address: {server_addr}:{server_port}

log:
  level: info
  format: text
  output: file
"""
    
    folder_path = Path(folder).expanduser()
    folder_path.mkdir(parents=True, exist_ok=True)
    
    config_file = folder_path / "client-config.yaml"
    with open(config_file, 'w') as f:
        f.write(config_content)
    
    print(f"✓ 配置文件已生成: {config_file}")

def main():
    print("生成客户端配置文件...\n")
    
    with open(CONFIG_FILE, 'r', encoding='utf-8') as f:
        config = yaml.safe_load(f)
    
    server_config = config['server']['tunnox-server']
    server_addr = server_config['domain']
    server_port = server_config['port']
    
    # 生成 listen-client 配置
    print("生成 listen-client 配置...")
    generate_client_config(
        config['listen-client']['folder'],
        config['listen-client']['client-id'],
        config['listen-client']['secret-key'],
        server_addr,
        server_port
    )
    
    # 生成 target-client 配置
    print("生成 target-client 配置...")
    generate_client_config(
        config['target-client']['folder'],
        config['target-client']['client-id'],
        config['target-client']['secret-key'],
        server_addr,
        server_port
    )
    
    print("\n✓ 所有配置文件生成完成!")

if __name__ == "__main__":
    main()
