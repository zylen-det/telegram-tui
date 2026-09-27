#!/bin/sh
# Record the synthetic-data client in a dedicated Kitty window on Hyprland.
# Usage: ./scripts/record-demo.sh [output.gif]
set -eu
cd "$(dirname "$0")/.."

for tool in kitty kitten hyprctl jq wf-recorder ffmpeg; do
    command -v "$tool" >/dev/null 2>&1 || { echo "Missing $tool" >&2; exit 1; }
done
[ -n "${WAYLAND_DISPLAY:-}" ] || { echo 'A Wayland session is required' >&2; exit 1; }

output=${1:-docs/assets/demo.gif}
[ ! -e "$output" ] || { echo "Refusing to overwrite $output" >&2; exit 1; }
[ -d "$(dirname "$output")" ] || { echo "Output directory does not exist: $(dirname "$output")" >&2; exit 1; }

work=$(mktemp -d)
socket="unix:$work/kitty.sock"
class="telegram-tui-demo-$$"
kitty_pid=
recorder_pid=
cleanup() {
    if [ -n "$recorder_pid" ]; then
        kill -INT "$recorder_pid" 2>/dev/null || true
        wait "$recorder_pid" 2>/dev/null || true
    fi
    if [ -n "$kitty_pid" ]; then
        kitten @ --to "$socket" send-key ctrl+c >/dev/null 2>&1 || true
        kill -TERM "$kitty_pid" 2>/dev/null || true
        wait "$kitty_pid" 2>/dev/null || true
    fi
    rm -rf "$work"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

kitty --class "$class" --listen-on "$socket" -o allow_remote_control=socket-only ./scripts/demo.sh &
kitty_pid=$!

# Do not record the build or an authorization prompt: wait for the fake chat.
ready=false
attempt=0
while [ "$attempt" -lt 100 ]; do
    kill -0 "$kitty_pid" 2>/dev/null || { echo 'Demo Kitty exited early' >&2; exit 1; }
    if kitten @ --to "$socket" get-text 2>/dev/null | grep -Fq 'Terminal Makers'; then
        ready=true
        break
    fi
    attempt=$((attempt + 1))
    sleep 0.2
done
[ "$ready" = true ] || { echo 'Demo chat did not become ready' >&2; exit 1; }

# Fullscreen only our Kitty window, not whichever window was previously focused.
hyprctl dispatch focuswindow "pid:$kitty_pid" >/dev/null
hyprctl -j activewindow | jq -e --arg class "$class" --argjson pid "$kitty_pid" \
    '.class == $class and .pid == $pid' >/dev/null || {
    echo 'Could not focus the demo Kitty window' >&2
    exit 1
}
if ! hyprctl -j activewindow | jq -e '.fullscreen == 2' >/dev/null; then
    hyprctl dispatch fullscreen 0 >/dev/null
fi
fullscreen=false
attempt=0
while [ "$attempt" -lt 20 ]; do
    if hyprctl -j activewindow | jq -e --arg class "$class" --argjson pid "$kitty_pid" \
        '.class == $class and .pid == $pid and .fullscreen == 2' >/dev/null; then
        fullscreen=true
        break
    fi
    attempt=$((attempt + 1))
    sleep 0.2
done
[ "$fullscreen" = true ] || { echo 'Demo Kitty did not enter fullscreen' >&2; exit 1; }
sleep 0.4 # Let the compositor finish resizing before choosing the capture rectangle.

# Take geometry only after fullscreen, from this exact Kitty process.
# Never fall back to capturing another window or an arbitrary desktop region.
geometry=$(hyprctl -j clients | jq -r --arg class "$class" --argjson pid "$kitty_pid" '
    [.[] | select(.class == $class and .pid == $pid and .mapped == true and .fullscreen == 2)
          | select(.size[0] > 0 and .size[1] > 0)]
    | if length == 1 then "\(.[0].at[0]),\(.[0].at[1]) \(.[0].size[0])x\(.[0].size[1])" else empty end
')
[ -n "$geometry" ] || { echo 'Could not identify the fullscreen demo geometry' >&2; exit 1; }

wf-recorder -g "$geometry" -f "$work/demo.mp4" -r 15 >/dev/null 2>&1 &
recorder_pid=$!
sleep 1
kill -0 "$recorder_pid" 2>/dev/null || { echo 'wf-recorder did not start' >&2; exit 1; }

# Start in the group, then demonstrate selecting messages up and down.
sleep 2.2
kitten @ --to "$socket" send-key tab
kitten @ --to "$socket" send-key k
sleep 0.75
kitten @ --to "$socket" send-key j
sleep 0.75
kitten @ --to "$socket" send-key k # Back to the photo message.
sleep 0.75

# Let the message action menu linger; briefly view the image at full size.
kitten @ --to "$socket" send-key enter
sleep 2
kitten @ --to "$socket" send-key v
sleep 0.8
kitten @ --to "$socket" send-key esc
sleep 0.3

# Show the action menu again, reply, and send a local mock message.
kitten @ --to "$socket" send-key enter
sleep 2
kitten @ --to "$socket" send-key r
sleep 0.5
kitten @ --to "$socket" send-text 'Love this image!'
sleep 0.5
kitten @ --to "$socket" send-key enter
sleep 1.8

# Open Mina's chat, show Info and her avatar, then return to the starting chat.
kitten @ --to "$socket" send-key esc
kitten @ --to "$socket" send-key h
kitten @ --to "$socket" send-key j
kitten @ --to "$socket" send-key enter
sleep 1.7
kitten @ --to "$socket" send-key f2
sleep 1.8
kitten @ --to "$socket" send-key enter
sleep 0.9
kitten @ --to "$socket" send-key esc
sleep 0.5
kitten @ --to "$socket" send-key f2
kitten @ --to "$socket" send-key k
kitten @ --to "$socket" send-key enter
sleep 2

kill -INT "$recorder_pid"
wait "$recorder_pid"
recorder_pid=
[ -s "$work/demo.mp4" ] || { echo 'Recording is empty' >&2; exit 1; }

# Palette-based conversion keeps the TUI colors legible without keeping an MP4.
ffmpeg -hide_banner -loglevel error -i "$work/demo.mp4" \
    -filter_complex '[0:v]fps=15,split[a][b];[a]palettegen=stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=3[v]' \
    -map '[v]' -t 25 -loop 0 "$work/demo.gif"
[ -s "$work/demo.gif" ] || { echo 'GIF conversion failed' >&2; exit 1; }
[ ! -e "$output" ] || { echo "Refusing to overwrite $output" >&2; exit 1; }
mv "$work/demo.gif" "$output"
printf 'Recorded %s\n' "$output"
