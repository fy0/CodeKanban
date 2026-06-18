#!/usr/bin/env python3
"""
自动发布所有 npm 包的脚本
"""
import json
import os
import subprocess
import sys
from pathlib import Path


MIN_NODE_VERSION_FOR_NPM_OIDC = (22, 14, 0)
MIN_NPM_VERSION_FOR_OIDC = (11, 5, 1)


def run_command(cmd: list[str], cwd: Path | None = None, shell: bool = False) -> int:
    """执行命令并实时输出"""
    print(f"\n[执行] {' '.join(cmd)}")
    print(f"[目录] {cwd if cwd else Path.cwd()}")
    print("-" * 60)

    # Windows 需要使用 shell=True
    is_windows = sys.platform.startswith('win')

    if is_windows:
        cmd_str = ' '.join(cmd)
        result = subprocess.run(cmd_str, cwd=cwd, shell=True)
    elif shell:
        result = subprocess.run(cmd[0], cwd=cwd, shell=True)
    else:
        result = subprocess.run(cmd, cwd=cwd)

    return result.returncode


def is_github_actions() -> bool:
    """检查是否在 GitHub Actions 环境中运行"""
    return os.getenv('GITHUB_ACTIONS') == 'true'


def command_output(cmd: list[str], cwd: Path | None = None) -> str:
    """执行命令并返回 stdout"""
    result = subprocess.run(cmd, cwd=cwd, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    if result.returncode != 0:
        raise RuntimeError(result.stdout.strip() or f"command failed: {' '.join(cmd)}")
    return result.stdout.strip()


def parse_version(value: str) -> tuple[int, int, int]:
    """解析 node/npm 版本号，忽略 v 前缀和预发布后缀"""
    cleaned = value.strip().lstrip('v').split('-', 1)[0]
    parts = cleaned.split('.')
    version = []
    for part in parts[:3]:
        try:
            version.append(int(part))
        except ValueError:
            version.append(0)
    while len(version) < 3:
        version.append(0)
    return tuple(version)


def check_github_actions_oidc(root_dir: Path) -> int:
    """校验 GitHub Actions OIDC 发布所需环境"""
    print("\n[认证] GitHub Actions 环境：使用 npm Trusted Publishing (OIDC)")

    request_token = os.getenv('ACTIONS_ID_TOKEN_REQUEST_TOKEN')
    request_url = os.getenv('ACTIONS_ID_TOKEN_REQUEST_URL')
    if not request_token or not request_url:
        print("[错误] 未检测到 GitHub Actions OIDC 请求环境变量")
        print("[提示] 请确认 workflow job 配置了 permissions.id-token: write，并使用 GitHub-hosted runner")
        return 1

    try:
        node_version_text = command_output(["node", "--version"], cwd=root_dir)
        npm_version_text = command_output(["npm", "--version"], cwd=root_dir)
    except RuntimeError as exc:
        print(f"[错误] 检查 node/npm 版本失败: {exc}")
        return 1

    node_version = parse_version(node_version_text)
    npm_version = parse_version(npm_version_text)
    print(f"[认证] Node.js: {node_version_text}")
    print(f"[认证] npm: {npm_version_text}")

    if node_version < MIN_NODE_VERSION_FOR_NPM_OIDC:
        required = ".".join(str(part) for part in MIN_NODE_VERSION_FOR_NPM_OIDC)
        print(f"[错误] npm OIDC 发布需要 Node.js >= {required}")
        return 1

    if npm_version < MIN_NPM_VERSION_FOR_OIDC:
        required = ".".join(str(part) for part in MIN_NPM_VERSION_FOR_OIDC)
        print(f"[错误] npm OIDC 发布需要 npm >= {required}")
        return 1

    print("[认证] OIDC 环境检查通过，npm publish 将在发布时换取短期令牌")
    return 0


def check_local_npm_auth(root_dir: Path) -> int:
    """本地发布仍使用当前 npm 登录状态"""
    print("\n[认证] 本地环境：检查 npm 登录状态...")
    auth_result = run_command(["npm", "whoami"], cwd=root_dir)
    if auth_result != 0:
        print("\n[错误] npm whoami 失败，请先运行 npm login 或改用 GitHub Actions OIDC 发布")
        return auth_result
    print("[认证] npm 登录状态正常")
    return 0


def main():
    root_dir = Path(__file__).parent.absolute()
    npm_packages_dir = root_dir / "npm-packages"

    print("=" * 60)
    print("发布所有 npm 包")
    print("=" * 60)

    if not npm_packages_dir.exists():
        print(f"\n[错误] {npm_packages_dir} 目录不存在")
        print("请先运行: python build-npm-multipackage.py")
        return 1

    # 读取主包名称
    main_package_json = root_dir / "package.json"
    if not main_package_json.exists():
        print(f"\n[错误] {main_package_json} 不存在")
        return 1

    with open(main_package_json, 'r', encoding='utf-8') as f:
        main_pkg = json.load(f)
        main_package_name = main_pkg.get('name', 'unknown')

    # 平台包列表
    platforms = [
        "win32-x64",
        "darwin-x64",
        "darwin-arm64",
        "linux-x64",
        "linux-arm64"
    ]

    if is_github_actions():
        auth_result = check_github_actions_oidc(root_dir)
    else:
        auth_result = check_local_npm_auth(root_dir)
    if auth_result != 0:
        return auth_result

    # 构建发布命令
    publish_cmd = ["npm", "publish", "--access", "public"]
    if is_github_actions():
        print("\n[信息] 使用 npm Trusted Publishing (OIDC) 发布")
    else:
        print("\n[信息] 使用本地 npm 登录凭据发布")

    # 发布所有平台包
    print("\n[步骤 1/2] 发布平台包")
    for platform in platforms:
        platform_dir = npm_packages_dir / platform

        if not platform_dir.exists():
            print(f"\n[警告] {platform_dir} 不存在，跳过")
            continue

        # 读取平台包名称
        platform_package_json = platform_dir / "package.json"
        if platform_package_json.exists():
            with open(platform_package_json, 'r', encoding='utf-8') as f:
                pkg = json.load(f)
                pkg_name = pkg.get('name', f'unknown-{platform}')
                pkg_version = pkg.get('version', 'unknown')
        else:
            pkg_name = f'unknown-{platform}'
            pkg_version = 'unknown'

        print(f"\n[调试] 准备发布: {pkg_name}@{pkg_version}")
        print(f"[调试] 包目录: {platform_dir}")
        print(f"[调试] 发布命令: {' '.join(publish_cmd)}")

        ret = run_command(
            publish_cmd,
            cwd=platform_dir
        )

        if ret != 0:
            print(f"\n[错误] {pkg_name} 发布失败")
            return ret

        print(f"[成功] {pkg_name} 发布成功")

    # 发布主包
    print("\n[步骤 2/2] 发布主包")
    print(f"\n发布 {main_package_name}...")

    ret = run_command(
        publish_cmd,
        cwd=root_dir
    )

    if ret != 0:
        print("\n[错误] 主包发布失败")
        return ret

    print("\n" + "=" * 60)
    print("所有包发布成功！")
    print("=" * 60)
    print("\n安装测试:")
    print(f"  npm install -g {main_package_name}")
    print("\n查看包:")
    print(f"  https://www.npmjs.com/package/{main_package_name}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
