#!/usr/bin/env python3
"""
自动发布所有 npm 包的脚本
"""
import json
import os
import subprocess
import sys
from pathlib import Path


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

    # 调试信息：检查 npm 认证
    print("\n[调试] 检查 npm 认证状态...")
    check_auth_cmd = ["npm", "whoami"]
    auth_result = run_command(check_auth_cmd, cwd=root_dir)

    if auth_result != 0:
        print("\n[警告] npm whoami 失败，可能未正确认证")
        print("[调试] 检查 .npmrc 配置...")
        npmrc_path = Path.home() / ".npmrc"
        if npmrc_path.exists():
            print(f"[调试] 找到 .npmrc: {npmrc_path}")
            # 不打印内容，避免泄露 token
        else:
            print(f"[警告] 未找到 .npmrc: {npmrc_path}")

        if is_github_actions():
            print("[调试] 在 GitHub Actions 中，检查 NODE_AUTH_TOKEN 环境变量...")
            if os.getenv('NODE_AUTH_TOKEN'):
                print("[调试] NODE_AUTH_TOKEN 已设置 (长度: {})".format(len(os.getenv('NODE_AUTH_TOKEN', ''))))
            else:
                print("[错误] NODE_AUTH_TOKEN 未设置！")
                return 1
    else:
        print("[调试] npm 认证成功")

    # 构建发布命令（在 GitHub Actions 中使用 provenance）
    publish_cmd = ["npm", "publish", "--access", "public"]
    if is_github_actions():
        publish_cmd.append("--provenance")
        print("\n[信息] 检测到 GitHub Actions 环境，将使用 --provenance 发布")

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
