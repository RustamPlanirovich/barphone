import 'package:flutter/material.dart';

/// Built-in pictures for buttons that have no app icon (keys, text, system actions).
/// Names match the agent's `glyph` field (see docs/protocol.md).
const _glyphs = <String, (IconData, Color)>{
  'keys': (Icons.keyboard_rounded, Color(0xFFFF7A45)),
  'text': (Icons.short_text_rounded, Color(0xFFFF7A45)),
  'media_play_pause': (Icons.play_arrow_rounded, Color(0xFF2EB8A6)),
  'media_next': (Icons.skip_next_rounded, Color(0xFF2EB8A6)),
  'media_prev': (Icons.skip_previous_rounded, Color(0xFF2EB8A6)),
  'media_stop': (Icons.stop_rounded, Color(0xFF2EB8A6)),
  'volume': (Icons.volume_up_rounded, Color(0xFF8E6BFF)),
  'volume_up': (Icons.volume_up_rounded, Color(0xFF8E6BFF)),
  'volume_down': (Icons.volume_down_rounded, Color(0xFF8E6BFF)),
  'mute': (Icons.volume_off_rounded, Color(0xFF8E6BFF)),
  'brightness': (Icons.brightness_6_rounded, Color(0xFFE0A63A)),
  'brightness_up': (Icons.brightness_high_rounded, Color(0xFFE0A63A)),
  'brightness_down': (Icons.brightness_low_rounded, Color(0xFFE0A63A)),
  'lock': (Icons.lock_rounded, Color(0xFF4F7CFF)),
  'sleep': (Icons.bedtime_rounded, Color(0xFF4F7CFF)),
  'display_off': (Icons.desktop_access_disabled_rounded, Color(0xFF4F7CFF)),
  'shutdown': (Icons.power_settings_new_rounded, Color(0xFFE5484D)),
  'restart': (Icons.restart_alt_rounded, Color(0xFFE5484D)),
  'folder': (Icons.folder_rounded, Color(0xFFE0A63A)),
  'macro': (Icons.bolt_rounded, Color(0xFFFF7A45)),
  'timer': (Icons.timer_outlined, Color(0xFF2EB8A6)),
  'cpu': (Icons.speed_rounded, Color(0xFF4F7CFF)),
  'trackpad': (Icons.mouse_rounded, Color(0xFF5FA8FF)),
  'command': (Icons.terminal_rounded, Color(0xFF2EB8A6)),
  'ram': (Icons.memory_rounded, Color(0xFF8E6BFF)),
};

bool hasGlyph(String? name) => name != null && _glyphs.containsKey(name);

class GlyphIcon extends StatelessWidget {
  const GlyphIcon(this.name, {super.key, required this.size});
  final String name;
  final double size;

  @override
  Widget build(BuildContext context) {
    final (icon, color) = _glyphs[name] ?? (Icons.bolt_rounded, const Color(0xFF5B616E));
    return Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      decoration: BoxDecoration(color: color.withValues(alpha: .9), borderRadius: BorderRadius.circular(size * .24)),
      child: Icon(icon, size: size * .62, color: Colors.white),
    );
  }
}
