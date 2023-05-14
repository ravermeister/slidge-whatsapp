"""
Hot-reloader for both go and python files (in the docker-compose dev setup)
"""

import os
import subprocess
import sys

from watchdog.observers import Observer
from watchdog.tricks import AutoRestartTrick, ShellCommandTrick

if __name__ == "__main__":
    observer = Observer()
    auto_restart = AutoRestartTrick(
        command=["python", "-m", "slidge"] + sys.argv[2:] if len(sys.argv) > 2 else [],
        patterns=["*.py"],
        ignore_patterns=["generated/*.py"],
    )
    gopy_cmd = "gopy build -output=generated -no-make=true ."
    gopy_build = ShellCommandTrick(
        shell_command='cd "$(dirname ${watch_src_path})" && '
        + gopy_cmd
        + ' && touch "$(dirname ${watch_src_path})/__init__.py"',
        patterns=["*.go"],
        ignore_patterns=["generated/*.go"],
        drop_during_process=True,
    )

    path = sys.argv[1] if len(sys.argv) > 1 else "."
    observer.schedule(auto_restart, path, recursive=True)
    observer.schedule(gopy_build, path, recursive=True)
    observer.start()

    try:
        for dirpath, _, filenames in os.walk(path):
            if "go.mod" in filenames:
                subprocess.run(gopy_cmd, shell=True, cwd=dirpath)
        auto_restart.start()
        while observer.is_alive():
            observer.join(1)
    finally:
        observer.stop()
        observer.join()