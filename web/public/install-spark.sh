#!/usr/bin/env bash
# Installs the Spark build bundled with the selected Campfire panel.
set -euo pipefail
umask 077

fail() { printf '%s\n' "$*" >&2; exit 1; }
panel= token= ca_file= ca_base64= source=
while (($#)); do
  case "$1" in
    --panel|--token|--ca-file|--ca-base64|--source)
      (($# >= 2)) || fail "Missing value for $1"
      case "$1" in
        --panel) panel=$2 ;;
        --source) source=$2 ;;
        --token) token=$2 ;;
        --ca-file) ca_file=$2 ;;
        --ca-base64) ca_base64=$2 ;;
      esac
      shift 2 ;;
    *) fail "Unknown option: $1" ;;
  esac
done
[[ $panel =~ ^https://[^[:space:]]+$ ]] || fail "An HTTPS agent address is required."
[[ $source =~ ^https://[^[:space:]]+$ ]] || fail "An HTTPS panel download address is required (--source)."
[[ $token =~ ^[A-Za-z0-9]+$ && ${#token} -le 256 ]] || fail "A valid enrollment token is required."
[[ -z $ca_file || -z $ca_base64 ]] || fail "Use either --ca-file or --ca-base64."
[[ ${#ca_base64} -le 65536 ]] || fail "The panel CA certificate is too large."
missing=()
for tool in curl tar sha256sum install systemctl docker uname mktemp chmod rm grep base64; do
  if ! command -v "$tool" >/dev/null; then missing+=("$tool"); fi
done
if ((${#missing[@]})); then
  printf 'Missing required dependencies:\n' >&2
  printf '  - %s\n' "${missing[@]}" >&2
  fail "Install all missing dependencies, then run this command again."
fi
[[ $EUID == 0 ]] || fail "Run this installer as root (sudo)."
[[ $(uname -s) == Linux ]] || fail "Spark installation requires Linux."
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) fail "Supported CPU architectures: amd64 and arm64." ;;
esac
[[ -d /run/systemd/system ]] || fail "This installer requires a running systemd host."
docker info >/dev/null 2>&1 || fail "Docker must be running before installing Spark."
[[ -z $ca_file || -f $ca_file ]] || fail "The panel CA certificate file does not exist."

stage=$(mktemp -d)
restart_previous=false
cleanup() {
  rm -rf -- "$stage"
  if $restart_previous; then systemctl start spark.service || true; fi
}
trap cleanup EXIT
if [[ -n $ca_base64 ]]; then
  ca_file=$stage/panel-ca.pem
  printf '%s' "$ca_base64" | base64 --decode > "$ca_file" || fail "Invalid panel CA certificate encoding."
  [[ -s $ca_file ]] || fail "The panel CA certificate is empty."
fi
version=$(curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --connect-timeout 15 --max-time 60 "$source/version") || fail "Could not read the panel's bundled Spark version."
[[ $version =~ ^[A-Za-z0-9][A-Za-z0-9.+_-]{0,127}$ ]] || fail "The panel returned an invalid Spark version."
archive=spark_linux_${arch}.tar.gz
for asset in "$archive" "$archive.sha256"; do
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --connect-timeout 15 --max-time 600 "$source/$version/$asset" --output "$stage/$asset" || fail "Could not download the panel's Spark package ($asset). Regenerate the command if Campfire was upgraded."
done
(
  cd "$stage"
  # Require the checksum to refer only to the expected archive.
  read -r digest filename < "$archive.sha256"
  [[ $digest =~ ^[a-fA-F0-9]{64}$ && $filename == "$archive" ]] || fail "Invalid release checksum file."
  printf '%s  %s\n' "$digest" "$archive" | sha256sum --check --status
) || fail "Release checksum verification failed."
tar -xzf "$stage/$archive" -C "$stage" ./spark ./deploy/systemd/spark.service
chmod 0755 "$stage/spark"
# Check compatibility before changing an existing installation.
"$stage/spark" --help 2>&1 | grep -q -- '-enroll-only' || fail "The bundled Spark does not support automatic installation."
"$stage/spark" --update-info | grep -Fq -- "\"version\":\"$version\"" || fail "The bundled Spark version does not match the panel."
install -d -m 0700 /var/lib/spark
if systemctl is-active --quiet spark.service; then
  restart_previous=true
  systemctl stop spark.service
fi
printf '%s' "$token" > "$stage/enrollment.token"
enroll_args=(--panel "$panel" --data /var/lib/spark --token-file "$stage/enrollment.token" --enroll-only)
if [[ -n $ca_file ]]; then enroll_args+=(--ca-file "$ca_file"); fi
"$stage/spark" "${enroll_args[@]}"
unset token
install -m 0755 "$stage/spark" /usr/local/bin/spark
# A dedicated drop-in uses the saved identity rather than retaining the token.
install -m 0644 "$stage/deploy/systemd/spark.service" /etc/systemd/system/spark.service
install -d -m 0755 /etc/systemd/system/spark.service.d
printf '[Service]\nEnvironmentFile=\nUnsetEnvironment=SPARK_PANEL SPARK_TOKEN_FILE SPARK_CA_FILE CAMPFIRE_ENROLLMENT_TOKEN\nEnvironment=SPARK_DATA=/var/lib/spark\n' > /etc/systemd/system/spark.service.d/installer.conf
systemctl daemon-reload
systemctl enable --now spark.service
restart_previous=false
systemctl is-active --quiet spark.service || fail "Spark could not start. Check journalctl -u spark.service."
printf 'Spark installed and enrolled. View logs with: journalctl -u spark.service -f\n'
