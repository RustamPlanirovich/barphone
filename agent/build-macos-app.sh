#!/bin/sh
# Builds dist/barphone.app for the architecture of the Mac it runs on.
# Needs Go 1.26+ and the Xcode command line tools (cgo draws the menu-bar icon).
set -eu
cd "$(dirname "$0")"

APP=dist/barphone.app
BUILD=$(git rev-list --count HEAD 2>/dev/null || echo 1)
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o "$APP/Contents/MacOS/barphone-agent" ./cmd/barphone-agent

go run ./tools/genicons -iconset "$TMP/barphone.iconset" >/dev/null
iconutil -c icns -o "$APP/Contents/Resources/barphone.icns" "$TMP/barphone.iconset"

# LSUIElement: menu-bar only, no Dock icon. The usage strings are what macOS shows when it
# asks for local network access (phones, mDNS) and for driving System Events (keystrokes).
cat > "$APP/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>barphone</string>
  <key>CFBundleDisplayName</key><string>barphone</string>
  <key>CFBundleIdentifier</key><string>io.barphone.agent</string>
  <key>CFBundleExecutable</key><string>barphone-agent</string>
  <key>CFBundleIconFile</key><string>barphone</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>1.0</string>
  <key>CFBundleVersion</key><string>$BUILD</string>
  <key>LSMinimumSystemVersion</key><string>12.0</string>
  <key>LSUIElement</key><true/>
  <key>NSHighResolutionCapable</key><true/>
  <key>NSLocalNetworkUsageDescription</key><string>barphone принимает нажатия кнопок с телефона по локальной сети.</string>
  <key>NSBonjourServices</key><array><string>_barphone._tcp</string></array>
  <key>NSAppleEventsUsageDescription</key><string>barphone нажимает сочетания клавиш и вводит текст по кнопкам с телефона.</string>
</dict>
</plist>
EOF
plutil -lint "$APP/Contents/Info.plist" >/dev/null

# Ad-hoc signature: enough to run on this Mac. macOS ties granted permissions to it, so
# after a rebuild Accessibility may need to be switched off and on again for barphone.
codesign --force --sign - "$APP"

echo "built $APP ($(lipo -archs "$APP/Contents/MacOS/barphone-agent"))"
