#!/usr/bin/env python3
"""
构建脚本：先构建前端，再将产物复制到 static 目录，最后构建 Go 程序
"""
import os
import shutil
import subprocess
import sys
from pathlib import Path


def run_command(cmd: list[str], cwd: Path | None = None, shell: bool = False) -> int:
    """执行命令并实时输出"""
    print(f"[执行] {' '.join(cmd) if not shell else cmd[0]}")
    if shell:
        # Windows 下需要 shell=True 来执行 pnpm
        result = subprocess.run(cmd[0], cwd=cwd, shell=True)
    else:
        result = subprocess.run(cmd, cwd=cwd)
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


def main():
    # 获取项目根目录
    root_dir = Path(__file__).parent.absolute()
    ui_dir = root_dir / "ui"
    dist_dir = ui_dir / "dist"
    static_dir = root_dir / "static"

    print("=" * 60)
    print("开始构建项目")
    print("=" * 60)

    # 步骤 1: 构建前端
    print("\n[步骤 1/3] 构建前端项目")
    if not ui_dir.exists():
        print(f"[错误] {ui_dir} 目录不存在")
        return 1

    # 根据操作系统选择是否使用 shell
    is_windows = sys.platform.startswith('win')

    if is_windows:
        ret = run_command(["pnpm build"], cwd=ui_dir, shell=True)
    else:
        ret = run_command(["pnpm", "build"], cwd=ui_dir)

    if ret != 0:
        print("[错误] 前端构建失败")
        return ret

    # 步骤 2: 复制产物到 static
    print("\n[步骤 2/3] 复制前端产物到 static 目录")
    clean_static_dir(static_dir)

    if not copy_dist_to_static(dist_dir, static_dir):
        return 1

    # 步骤 3: 构建 Go 程序
    print("\n[步骤 3/3] 构建 Go 程序（优化模式）")
    exe_name = "CodeKanban.exe" if is_windows else "CodeKanban"

    # 编译优化选项：
    # -ldflags="-s -w"  去除调试信息和符号表，减小体积
    # -trimpath         移除文件系统路径，提升安全性和可重现性
    build_cmd = [
        "go", "build",
        "-ldflags=-s -w",
        "-trimpath",
        "-o", exe_name
    ]

    ret = run_command(build_cmd, cwd=root_dir)

    if ret != 0:
        print("[错误] Go 构建失败")
        return ret

    print("\n" + "=" * 60)
    print("构建成功！")
    print("=" * 60)

    # 输出构建产物信息
    exe_path = root_dir / exe_name
    if exe_path.exists():
        size_mb = exe_path.stat().st_size / (1024 * 1024)
        print(f"可执行文件: {exe_path}")
        print(f"文件大小: {size_mb:.2f} MB")

    return 0


if __name__ == "__main__":
    sys.exit(main())
