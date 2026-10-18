"""Exercise production orchestration without a server, registry, or credentials."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent
OLD = "registry.test/samen-klaar@sha256:" + "a" * 64
NEW = "registry.test/samen-klaar@sha256:" + "b" * 64


class DeploymentFailureTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / "docker").mkdir()
        shutil.copy(ROOT / "docker/deploy.sh", self.root / "docker/deploy.sh")
        self.state = self.root / ".deploy/prod"
        self.state.mkdir(parents=True)
        (self.root / ".deploy/prod.env").write_text(
            f"APP_IMAGE={NEW}\nDOMAIN=demo.test\nACME_EMAIL=deploy@demo.test\n"
        )
        (self.state / "current-image").write_text(OLD + "\n")
        self.bin = self.root / "bin"
        self.bin.mkdir()
        docker = self.bin / "docker"
        docker.write_text('''#!/bin/sh
printf '%s %s\n' "$APP_IMAGE" "$*" >> "$COMMAND_LOG"
case "$*" in
  *check-config*) [ "$FAIL_STEP" != preflight ] ;;
  *"run --rm --no-deps migrate") [ "$FAIL_STEP" != migration ] ;;
  *"up -d --no-deps --no-build"*)
    if [ "$FAIL_STEP" = app ] && [ "$APP_IMAGE" = "$NEW_IMAGE" ]; then exit 1; fi ;;
  *pg_dump*) printf 'test-backup' ;;
esac
''')
        docker.chmod(0o755)
        curl = self.bin / "curl"
        curl.write_text("#!/bin/sh\nexit 0\n")
        curl.chmod(0o755)
        self.log = self.root / "commands.log"
        self.env = {
            **os.environ,
            "PATH": str(self.bin) + os.pathsep + os.environ["PATH"],
            "COMMAND_LOG": str(self.log), "NEW_IMAGE": NEW, "FAIL_STEP": "",
        }
        self.env.pop("APP_IMAGE", None)

    def run_deploy(self, fail_step="", action="deploy"):
        result = subprocess.run(
            ["sh", "docker/deploy.sh", action, "prod"], cwd=self.root,
            env={**self.env, "FAIL_STEP": fail_step}, capture_output=True, text=True,
        )
        self.assertFalse((self.state / "lock").exists(), result.stderr)
        return result

    def test_preflight_and_migration_failure_preserve_running_release(self):
        for step in ("preflight", "migration"):
            with self.subTest(step=step):
                self.log.write_text("")
                result = self.run_deploy(step)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("up -d --no-deps --no-build", self.log.read_text())
                self.assertEqual((self.state / "current-image").read_text().strip(), OLD)

    def test_failed_replacement_restores_old_image_and_returns_failure(self):
        result = self.run_deploy("app")
        self.assertNotEqual(result.returncode, 0)
        commands = self.log.read_text().splitlines()
        replacements = [line for line in commands if "up -d --no-deps --no-build" in line]
        self.assertEqual(len(replacements), 2)
        self.assertTrue(replacements[0].startswith(NEW))
        self.assertTrue(replacements[1].startswith(OLD))
        self.assertEqual((self.state / "current-image").read_text().strip(), OLD)
        self.assertIn("Previous app image restored", result.stderr)

    def test_local_credentials_added_after_initialization_fill_only_empty_settings(self):
        (self.root / ".deploy/local").mkdir()
        (self.root / ".deploy/local.env").write_text("APP_IMAGE=samen-klaar-local:dev\n")
        local = self.root / ".secrets/local"
        local.mkdir(parents=True)
        (local / "postgres-password").write_text("existing-password")
        active = local / "app.env"
        active.write_text(
            "AUTH0_ISSUER_URL=\nAUTH0_CLIENT_ID=already-configured\nAUTH0_CLIENT_SECRET=\n"
            "DATABASE_URL=postgres://relay@db/relay\nAPP_BASE_URL=http://localhost:8090\n"
            "AUTH0_REDIRECT_URI=http://localhost:8090/api/auth/callback\n"
        )
        source = self.root / ".env"
        source.write_text(
            "AUTH0_ISSUER_URL=https://tenant.test/\nAUTH0_CLIENT_ID=imported-client\n"
            "AUTH0_CLIENT_SECRET=dummy-local-secret\nDATABASE_URL=wrong-database\n"
            "APP_BASE_URL=http://wrong-origin\nAUTH0_REDIRECT_URI=http://wrong-callback\n"
        )
        result = subprocess.run(
            ["sh", "docker/deploy.sh", "deploy", "local"], cwd=self.root,
            env=self.env, capture_output=True, text=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        settings = dict(line.split("=", 1) for line in active.read_text().splitlines())
        self.assertEqual(settings["AUTH0_ISSUER_URL"], "https://tenant.test/")
        self.assertEqual(settings["AUTH0_CLIENT_ID"], "already-configured")
        self.assertEqual(settings["AUTH0_CLIENT_SECRET"], "dummy-local-secret")
        self.assertEqual(settings["DATABASE_URL"], "postgres://relay@db/relay")
        self.assertEqual(settings["APP_BASE_URL"], "http://localhost:8090")
        self.assertEqual(settings["AUTH0_REDIRECT_URI"], "http://localhost:8090/api/auth/callback")
        self.assertNotIn("dummy-local-secret", result.stdout + result.stderr)
        # An empty import file must not erase the active configuration.
        before = active.read_text()
        source.write_text("")
        result = subprocess.run(
            ["sh", "docker/deploy.sh", "validate", "local"], cwd=self.root,
            env=self.env, capture_output=True, text=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(active.read_text(), before)

    def test_success_records_release_and_manual_rollback_swaps_it(self):
        result = self.run_deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.state / "current-image").read_text().strip(), NEW)
        self.assertEqual((self.state / "previous-image").read_text().strip(), OLD)
        result = self.run_deploy(action="rollback")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.state / "current-image").read_text().strip(), OLD)
        self.assertEqual((self.state / "previous-image").read_text().strip(), NEW)


class PublicationGateTests(unittest.TestCase):
    def test_flow_gate_runs_before_any_registry_operation(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "docker").mkdir()
            shutil.copy(ROOT / "docker/publish.sh", root / "docker/publish.sh")
            (root / "docker/verify-flow.sh").write_text(
                '#!/bin/sh\necho gate >> "$COMMAND_LOG"\nexit "$GATE_STATUS"\n'
            )
            bin_dir = root / "bin"
            bin_dir.mkdir()
            (bin_dir / "git").write_text('#!/bin/sh\n[ "$1" = status ] || echo fixture-revision\n')
            (bin_dir / "docker").write_text('#!/bin/sh\necho registry >> "$COMMAND_LOG"\nexit 23\n')
            for executable in bin_dir.iterdir():
                executable.chmod(0o755)
            log = root / "commands.log"
            for status in ("1", "0"):
                with self.subTest(gate_status=status):
                    log.write_text("")
                    result = subprocess.run(
                        ["sh", "docker/publish.sh"], cwd=root, capture_output=True, text=True,
                        env={**os.environ, "PATH": str(bin_dir) + os.pathsep + os.environ["PATH"],
                             "IMAGE_REPO": "registry.test/samen-klaar", "COMMAND_LOG": str(log), "GATE_STATUS": status},
                    )
                    self.assertEqual(log.read_text().splitlines(), ["gate"] if status == "1" else ["gate", "registry"])
                    self.assertEqual(result.returncode, 1 if status == "1" else 23, result.stderr)


if __name__ == "__main__":
    unittest.main()
