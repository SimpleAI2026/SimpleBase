"""Harbor BaseInstalledAgent：把已编译的 gocloudagent 放进任务容器。

供应商密钥留在宿主机的 simplebase 进程里。容器只拿到项目 API token。
"""

from __future__ import annotations

import json
import os
from pathlib import Path

from harbor.agents.installed.base import BaseInstalledAgent
from harbor.agents.options import InstalledAgentOptions
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext


class CloudAgentInstalled(BaseInstalledAgent):
    options_model = InstalledAgentOptions

    @staticmethod
    def name() -> str:
        return "gocloudagent"

    def get_version_command(self) -> str | None:
        return "/usr/local/bin/gocloudagent --version"

    def parse_version(self, stdout: str) -> str:
        lines = [line.strip() for line in stdout.splitlines() if line.strip()]
        return lines[-1] if lines else stdout.strip()

    async def install(self, environment: BaseEnvironment) -> None:
        binary = os.environ.get("GOCLOUDAGENT_BIN", "")
        if not binary or not Path(binary).is_file():
            raise RuntimeError("GOCLOUDAGENT_BIN is missing")
        await self.ensure_system_dependencies(environment, ("ca_certificates",))
        await self.exec_as_root(
            environment,
            "mkdir -p /usr/local/bin /logs/agent /tmp/gocloudagent-home && chmod 777 /logs/agent /tmp/gocloudagent-home",
        )
        sysroot = Path(os.environ.get("GOCLOUDAGENT_SYSROOT", ""))
        libs = [
            sysroot / name
            for name in (
                "ld-linux-x86-64.so.2",
                "libc.so.6",
                "libm.so.6",
                "libstdc++.so.6",
                "libgcc_s.so.1",
            )
        ]
        # 宿主机是 glibc 2.39，binary 需要 fmod@GLIBC_2.38。bookworm / Ubuntu 22.04
        # 的任务镜像没有这个符号。把宿主机的 loader 和库一并放进容器，不在容器里编译。
        if sysroot.is_dir() and all(path.is_file() for path in libs):
            await self.exec_as_root(
                environment,
                "mkdir -p /usr/local/lib/gocloudagent/lib",
            )
            await environment.upload_file(binary, "/usr/local/lib/gocloudagent/gocloudagent.bin")
            for path in libs:
                await environment.upload_file(path, f"/usr/local/lib/gocloudagent/lib/{path.name}")
            await self._upload_config_text(
                environment,
                content=(
                    "#!/bin/sh\n"
                    "export GODEBUG=\"${GODEBUG:+$GODEBUG,}netdns=go\"\n"
                    "exec /usr/local/lib/gocloudagent/lib/ld-linux-x86-64.so.2 "
                    "--library-path /usr/local/lib/gocloudagent/lib "
                    "/usr/local/lib/gocloudagent/gocloudagent.bin \"$@\"\n"
                ),
                remote_path="/usr/local/bin/gocloudagent",
                filename="gocloudagent.sh",
            )
            version_cmd = (
                "chmod 755 /usr/local/bin/gocloudagent /usr/local/lib/gocloudagent/gocloudagent.bin "
                "/usr/local/lib/gocloudagent/lib/ld-linux-x86-64.so.2 "
                "&& /usr/local/bin/gocloudagent --version"
            )
        else:
            await environment.upload_file(binary, "/usr/local/bin/gocloudagent")
            version_cmd = "chmod 755 /usr/local/bin/gocloudagent && /usr/local/bin/gocloudagent --version"
        try:
            result = await self.exec_as_root(environment, version_cmd)
        except RuntimeError as exc:
            text = str(exc)
            if "No such file or directory" in text or "not found" in text.lower() or "GLIBC_" in text:
                raise RuntimeError("unsupported_libc") from exc
            raise
        text = f"{getattr(result, 'stdout', '') or ''}\n{getattr(result, 'stderr', '') or ''}"
        if "No such file or directory" in text or "not found" in text.lower() or "GLIBC_" in text:
            raise RuntimeError("unsupported_libc")

    async def run(
        self,
        instruction: str,
        environment: BaseEnvironment,
        context: AgentContext,
    ) -> None:
        port = os.environ.get("GOCLOUDAGENT_PORT", "18080")
        if not port.isdigit():
            raise RuntimeError("GOCLOUDAGENT_PORT is not numeric")
        for key in ("GOCLOUDAGENT_TOKEN", "GOCLOUDAGENT_PROJECT_ID", "GOCLOUDAGENT_MODEL"):
            if not os.environ.get(key):
                raise RuntimeError(f"missing {key}")
        await self._upload_config_text(
            environment,
            content=instruction,
            remote_path="/tmp/task-instruction.txt",
            filename="instruction.txt",
        )
        command = (
            "hex=$(awk '$2==\"00000000\" {print $3; exit}' /proc/net/route); "
            "gw=$(printf '%d.%d.%d.%d' \"0x${hex:6:2}\" \"0x${hex:4:2}\" \"0x${hex:2:2}\" \"0x${hex:0:2}\"); "
            "export GOCLOUDAGENT_ENDPOINT=\"http://${gw}:${GOCLOUDAGENT_PORT}\"; "
            "mkdir -p \"$GOCLOUDAGENT_HOME\"; "
            "/usr/local/bin/gocloudagent --json --yolo --cwd \"$PWD\" -p \"$(cat /tmp/task-instruction.txt)\" "
            "> /logs/agent/gocloudagent.ndjson 2> /logs/agent/gocloudagent.err; "
            "echo $? > /logs/agent/exit_code; "
            "exit 0"
        )
        env = {
            "GOCLOUDAGENT_PORT": port,
            "GOCLOUDAGENT_TOKEN": os.environ["GOCLOUDAGENT_TOKEN"],
            "GOCLOUDAGENT_PROJECT_ID": os.environ["GOCLOUDAGENT_PROJECT_ID"],
            "GOCLOUDAGENT_MODEL": os.environ["GOCLOUDAGENT_MODEL"],
            "GOCLOUDAGENT_MAX_ITERATIONS": os.environ.get("GOCLOUDAGENT_MAX_ITERATIONS", "40"),
            "GOCLOUDAGENT_HOME": "/tmp/gocloudagent-home",
            "GOCLOUDAGENT_YOLO": "1",
        }
        await self.exec_as_agent(environment, command, env=env)
        self._fill_context(context)

    def populate_context_post_run(self, context: AgentContext) -> None:
        self._fill_context(context)

    def _fill_context(self, context: AgentContext) -> None:
        matches = list(Path(self.logs_dir).rglob("gocloudagent.ndjson"))
        if not matches:
            return
        steps = 0
        seen: set[str] = set()
        end_reason = ""
        for line in matches[0].read_text(encoding="utf-8", errors="replace").splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            if not isinstance(event, dict):
                continue
            if event.get("type") == "tool_call":
                call_id = str(event.get("call_id") or "")
                if call_id and call_id in seen:
                    continue
                if call_id:
                    seen.add(call_id)
                steps += 1
            if event.get("type") == "end":
                end_reason = str(event.get("message") or "stop")[:180]
        meta = dict(context.metadata or {})
        meta["steps"] = steps
        if end_reason:
            meta["end_reason"] = end_reason
        context.metadata = meta
