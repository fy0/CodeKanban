#!/usr/bin/env python3
"""
NPM 多包发布构建脚本：为每个平台创建独立的 npm 包
采用 esbuild 风格的发布策略
"""
import os
import shutil
import subprocess
import sys
import json
from pathlib import Path


def run_command(cmd: list[str], cwd: Path | None = None, shell: bool = False, env: dict = None) -> int:
    """执行命令并实时输出"""
    print(f"[执行] {' '.join(cmd) if not shell else cmd[0]}")
    if shell:
        result = subprocess.run(cmd[0], cwd=cwd, shell=True, env=env)
    else:
        result = subprocess.run(cmd, cwd=cwd, env=env)
    return result.returncode


def clean_static_dir(static_dir: Path):
    """清空 static 目录但保留 README.md"""
    print(f"[清理] 清空 {static_dir} 目录（保留 README.md）")

    if not static_dir.exists():
        static_dir.mkdir(parents=True)
        print(f"[创建] {static_dir} 目录")
        return

    for item in static_dir.iterdir():
        if item.name == "README.md":
            continue

        if item.is_file():
            item.unlink()
            print(f"[删除] {item}")
        elif item.is_dir():
            shutil.rmtree(item)
            print(f"[删除] {item}/")


def copy_dist_to_static(dist_dir: Path, static_dir: Path):
    """复制 ui/dist 到 static 目录"""
    print(f"[复制] {dist_dir} -> {static_dir}")

    if not dist_dir.exists():
        print(f"[错误] {dist_dir} 不存在，请先构建前端")
        return False

    for item in dist_dir.iterdir():
        dest = static_dir / item.name

        if item.is_file():
            shutil.copy2(item, dest)
            print(f"  复制文件: {item.name}")
        elif item.is_dir():
            if dest.exists():
                shutil.rmtree(dest)
            shutil.copytree(item, dest)
            print(f"  复制目录: {item.name}/")

    return True


def build_go_multiplatform(root_dir: Path, npm_packages_dir: Path, version_main: str = "", version_prerelease: str = "", version_build_metadata: str = "", app_channel: str = ""):
    """构建多平台版本（每个平台一个 npm 包）"""
    print("\n[步骤 3/5] 构建多平台 Go 程序")

    # Go -> npm 平台映射
    platforms = [
        ("linux", "amd64", "linux", "x64"),
        ("linux", "arm64", "linux", "arm64"),
        ("darwin", "amd64", "darwin", "x64"),
        ("darwin", "arm64", "darwin", "arm64"),
        ("windows", "amd64", "win32", "x64"),
    ]

    success_count = 0
    total_size = 0

    # 构建 ldflags，注入版本信息
    ldflags_parts = ["-s", "-w"]
    if version_main:
        ldflags_parts.append(f"-X 'main.VERSION_MAIN={version_main}'")
    # 总是注入，空字符串会覆盖默认的 -alpha
    ldflags_parts.append(f"-X 'main.VERSION_PRERELEASE={version_prerelease}'")
    if version_build_metadata:
        ldflags_parts.append(f"-X 'main.VERSION_BUILD_METADATA={version_build_metadata}'")
    if app_channel:
        ldflags_parts.append(f"-X 'main.APP_CHANNEL={app_channel}'")

    ldflags = " ".join(ldflags_parts)
    print(f"版本注入信息: VERSION_MAIN={version_main}, PRERELEASE={version_prerelease}, BUILD_METADATA={version_build_metadata}, CHANNEL={app_channel}")

    for goos, goarch, npm_os, npm_arch in platforms:
        print(f"\n构建 {goos}/{goarch} -> {npm_os}-{npm_arch}...")

        # 输出文件名
        output_name = "codekanban"
        if goos == "windows":
            output_name += ".exe"

        # 为平台创建包目录
        platform_package_dir = npm_packages_dir / f"{npm_os}-{npm_arch}"
        platform_package_dir.mkdir(parents=True, exist_ok=True)
        output_path = platform_package_dir / output_name

        # 设置环境变量
        env = os.environ.copy()
        env["GOOS"] = goos
        env["GOARCH"] = goarch
        env["CGO_ENABLED"] = "0"

        build_cmd = [
            "go", "build",
            f"-ldflags={ldflags}",
            "-trimpath",
            "-o", str(output_path),
            "."
        ]

        result = subprocess.run(build_cmd, cwd=root_dir, env=env)

        if result.returncode != 0:
            print(f"[错误] {goos}/{goarch} 构建失败")
            return result.returncode

        # 输出文件大小
        if output_path.exists():
            size_mb = output_path.stat().st_size / (1024 * 1024)
            total_size += output_path.stat().st_size
            print(f"  OK: {output_name} ({size_mb:.2f} MB)")
            success_count += 1

    print(f"\n成功构建 {success_count}/{len(platforms)} 个平台")
    print(f"总大小: {total_size / (1024 * 1024):.2f} MB")
    return 0


def create_platform_packages(root_dir: Path, npm_packages_dir: Path, version: str, base_name: str):
    """为每个平台创建 package.json 和标记文件"""
    print("\n[步骤 4/5] 创建平台包配置")

    platforms = [
        ("win32", "x64"),
        ("darwin", "x64"),
        ("darwin", "arm64"),
        ("linux", "x64"),
        ("linux", "arm64"),
    ]

    # 根据主包名决定平台包命名规则
    # 如果主包有 scope（如 @org/name），平台包用 @org/name-platform
    # 如果主包无 scope（如 name），平台包用 @name/platform
    if base_name.startswith('@'):
        # 主包有 scope，平台包沿用：@org/name-platform
        platform_name_template = f"{base_name}-{{platform}}"
    else:
        # 主包无 scope，平台包使用：@name/platform
        platform_name_template = f"@{base_name}/{{platform}}"

    for npm_os, npm_arch in platforms:
        platform_dir = npm_packages_dir / f"{npm_os}-{npm_arch}"

        # 创建 .npm-global 标记文件
        marker_file = platform_dir / ".npm-global"
        marker_file.touch()
        print(f"  创建标记文件: {marker_file.name}")

        # 创建 package.json
        platform_key = f"{npm_os}-{npm_arch}"
        package_json = {
            "name": platform_name_template.format(platform=platform_key),
            "version": version,
            "description": f"Platform-specific binary for {npm_os}-{npm_arch}. Install 'codekanban' instead: https://www.npmjs.com/package/codekanban",
            "os": [npm_os],
            "cpu": [npm_arch],
            "homepage": "https://www.npmjs.com/package/codekanban",
            "repository": {
                "type": "git",
                "url": "https://github.com/fy0/CodeKanban"
            },
            "author": "fy0",
            "license": "Apache-2.0"
        }

        package_json_path = platform_dir / "package.json"
        with open(package_json_path, 'w', encoding='utf-8') as f:
            json.dump(package_json, f, indent=2, ensure_ascii=False)

        print(f"  创建: {package_json['name']}")

    return 0


def create_main_package(root_dir: Path, version: str, base_name: str):
    """创建主包的 package.json 和启动脚本"""
    print("\n[步骤 5/5] 创建主包")

    # README.md 已经是英文版，无需复制

    # 根据主包名决定平台包命名规则（与 create_platform_packages 保持一致）
    if base_name.startswith('@'):
        # 主包有 scope，平台包沿用：@org/name-platform
        platform_name_template = f"{base_name}-{{platform}}"
    else:
        # 主包无 scope，平台包使用：@name/platform
        platform_name_template = f"@{base_name}/{{platform}}"

    # 平台列表
    platforms = ["win32-x64", "darwin-x64", "darwin-arm64", "linux-x64", "linux-arm64"]

    # 生成平台映射
    platforms_mapping = "\n".join([
        f"  '{p}': '{platform_name_template.format(platform=p)}'," for p in platforms
    ])

    # 创建 bin 目录
    bin_dir = root_dir / "npm-bin"
    bin_dir.mkdir(exist_ok=True)

    # 创建启动脚本
    launcher_script = f'''#!/usr/bin/env node
const {{ spawn }} = require('child_process');
const path = require('path');

// 平台映射
const PLATFORMS = {{
{platforms_mapping}
}};

const platform = process.platform;
const arch = process.arch;
const platformKey = `${{platform}}-${{arch}}`;
const packageName = PLATFORMS[platformKey];

if (!packageName) {{
  console.error(`Unsupported platform: ${{platformKey}}`);
  console.error('Supported platforms:', Object.keys(PLATFORMS).join(', '));
  process.exit(1);
}}

// 查找二进制文件
let binPath;
try {{
  const packagePath = require.resolve(packageName + '/package.json');
  const packageDir = path.dirname(packagePath);
  const binName = platform === 'win32' ? 'codekanban.exe' : 'codekanban';
  binPath = path.join(packageDir, binName);
}} catch (e) {{
  console.error(`Failed to find binary for ${{platformKey}}`);
  console.error(`Make sure ${{packageName}} is installed`);
  console.error('');
  console.error('Try one of the following:');
  console.error('  1. Uninstall and reinstall with npm:');
  console.error('     npm uninstall -g {base_name}');
  console.error('     npm install -g {base_name}');
  console.error('  2. Or uninstall and reinstall with pnpm:');
  console.error('     pnpm uninstall -g {base_name}');
  console.error('     pnpm install -g {base_name}');
  process.exit(1);
}}

// 运行二进制
const child = spawn(binPath, process.argv.slice(2), {{
  stdio: 'inherit',
  windowsHide: false
}});

child.on('error', (err) => {{
  console.error('Failed to start binary:', err.message);
  process.exit(1);
}});

child.on('exit', (code) => {{
  process.exit(code || 0);
}});
'''

    launcher_path = bin_dir / "codekanban.js"
    with open(launcher_path, 'w', encoding='utf-8', newline='\n') as f:
        f.write(launcher_script)

    print(f"  创建启动脚本: {launcher_path}")

    # 生成 optionalDependencies（使用与平台包相同的命名规则）
    optional_deps = {
        platform_name_template.format(platform=p): version
        for p in platforms
    }

    # 创建主包 package.json
    main_package_json = {
        "name": base_name,
        "version": version,
        "description": "An auxiliary programming tool for the AI era, helping you speed up 10x.",
        "bin": {
            "codekanban": "npm-bin/codekanban.js"
        },
        "optionalDependencies": optional_deps,
        "keywords": [
            "ai",
            "coding",
            "kanban",
            "terminal",
            "productivity",
            "developer-tools",
            "worktree",
            "git"
        ],
        "author": "fy0",
        "license": "Apache-2.0",
        "repository": {
            "type": "git",
            "url": "https://github.com/fy0/CodeKanban"
        },
        "homepage": "https://github.com/fy0/CodeKanban#readme",
        "engines": {
            "node": ">=14.0.0"
        }
    }

    main_package_path = root_dir / "package.json"
    with open(main_package_path, 'w', encoding='utf-8') as f:
        json.dump(main_package_json, f, indent=2, ensure_ascii=False)

    print(f"  更新主包: {main_package_path}")

    return 0


def main():
    import argparse

    parser = argparse.ArgumentParser(description='NPM 多包发布构建')
    parser.add_argument('--version', type=str, default='0.0.3', help='版本号')
    parser.add_argument('--package-name', type=str, default='codekanban', help='包名')
    parser.add_argument('--version-main', type=str, default='', help='主版本号（注入到二进制）')
    parser.add_argument('--version-prerelease', type=str, default='', help='预发布版本（注入到二进制）')
    parser.add_argument('--version-build-metadata', type=str, default='', help='构建元数据（注入到二进制）')
    parser.add_argument('--app-channel', type=str, default='', help='发布渠道（注入到二进制）')
    args = parser.parse_args()

    root_dir = Path(__file__).parent.absolute()
    ui_dir = root_dir / "ui"
    dist_dir = ui_dir / "dist"
    static_dir = root_dir / "static"
    npm_packages_dir = root_dir / "npm-packages"

    print("=" * 60)
    print("NPM 多包发布构建（esbuild 风格）")
    print("=" * 60)
    print(f"版本: {args.version}")
    print(f"包名: {args.package_name}")
    if args.version_main:
        print(f"版本信息: {args.version_main}{args.version_prerelease}{args.version_build_metadata}")
        print(f"发布渠道: {args.app_channel}")

    # 清理旧的包目录
    if npm_packages_dir.exists():
        print(f"\n[清理] 删除旧的包目录: {npm_packages_dir}")
        shutil.rmtree(npm_packages_dir)
    npm_packages_dir.mkdir()

    # 步骤 1: 构建前端
    print("\n[步骤 1/5] 构建前端项目")
    if not ui_dir.exists():
        print(f"[错误] {ui_dir} 目录不存在")
        return 1

    is_windows = sys.platform.startswith('win')
    if is_windows:
        ret = run_command(["pnpm build"], cwd=ui_dir, shell=True)
    else:
        ret = run_command(["pnpm", "build"], cwd=ui_dir)

    if ret != 0:
        print("[错误] 前端构建失败")
        return ret

    # 步骤 2: 复制产物到 static
    print("\n[步骤 2/5] 复制前端产物到 static 目录")
    clean_static_dir(static_dir)
    if not copy_dist_to_static(dist_dir, static_dir):
        return 1

    # 步骤 3: 构建多平台 Go 程序
    version = args.version
    base_name = args.package_name

    ret = build_go_multiplatform(
        root_dir,
        npm_packages_dir,
        version_main=args.version_main,
        version_prerelease=args.version_prerelease,
        version_build_metadata=args.version_build_metadata,
        app_channel=args.app_channel
    )
    if ret != 0:
        return ret

    # 步骤 4: 创建平台包配置
    ret = create_platform_packages(root_dir, npm_packages_dir, version, base_name)
    if ret != 0:
        return ret

    # 步骤 5: 创建主包
    ret = create_main_package(root_dir, version, base_name)
    if ret != 0:
        return ret

    print("\n" + "=" * 60)
    print("构建成功！")
    print("=" * 60)
    print("\n接下来的步骤:")
    print("  1. 发布所有平台包:")
    for platform in ["win32-x64", "darwin-x64", "darwin-arm64", "linux-x64", "linux-arm64"]:
        print(f"     cd npm-packages/{platform} && npm publish --access public")
    print("  2. 发布主包:")
    print("     npm publish --access public")
    print("\n或使用自动发布脚本:")
    print("     python publish-all-packages.py")

    return 0


if __name__ == "__main__":
    sys.exit(main())
