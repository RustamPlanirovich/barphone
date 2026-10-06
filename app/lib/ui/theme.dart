import 'package:flutter/material.dart';

import '../protocol/link.dart';

abstract final class C {
  static const bg = Color(0xFF0B0C0F);
  static const surface = Color(0xFF171A21);
  static const tile = Color(0xFF1E222B);
  static const tilePressed = Color(0xFF2A303B);
  static const border = Color(0xFF272C36);
  static const text = Color(0xFFE7E9EE);
  static const muted = Color(0xFF8B919D);
  static const accent = Color(0xFFFF7A45);
  static const accentSoft = Color(0xFFFFB38F);
  static const ok = Color(0xFF3CCF7A);
  static const danger = Color(0xFFFF5D5D);
  static const warn = Color(0xFFF5B83D);
}

ThemeData buildTheme() {
  final base = ThemeData(
    useMaterial3: true,
    brightness: Brightness.dark,
    colorScheme: ColorScheme.fromSeed(seedColor: C.accent, brightness: Brightness.dark, primary: C.accent, surface: C.surface),
    scaffoldBackgroundColor: C.bg,
  );
  return base.copyWith(
    textTheme: base.textTheme.apply(bodyColor: C.text, displayColor: C.text),
    snackBarTheme: const SnackBarThemeData(
      behavior: SnackBarBehavior.floating,
      backgroundColor: C.surface,
      contentTextStyle: TextStyle(color: C.text),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        backgroundColor: C.accent,
        foregroundColor: const Color(0xFF1A0D07),
        textStyle: const TextStyle(fontWeight: FontWeight.w600, fontSize: 15),
        padding: const EdgeInsets.symmetric(horizontal: 22, vertical: 14),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
      ),
    ),
    dialogTheme: const DialogThemeData(backgroundColor: C.surface),
    bottomSheetTheme: const BottomSheetThemeData(backgroundColor: C.surface),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: C.tile,
      border: OutlineInputBorder(borderRadius: BorderRadius.circular(12), borderSide: BorderSide.none),
    ),
  );
}

Color statusColor(LinkStatus s) => switch (s) {
  LinkStatus.online => C.ok,
  LinkStatus.connecting => C.warn,
  LinkStatus.offline => C.muted,
  LinkStatus.unauthorized => C.danger,
};

String statusText(LinkStatus s) => switch (s) {
  LinkStatus.online => 'на связи',
  LinkStatus.connecting => 'подключение…',
  LinkStatus.offline => 'не в сети',
  LinkStatus.unauthorized => 'нужно подключить заново',
};

String launchError(String? code) => switch (code) {
  'offline' => 'Нет связи с компьютером',
  'timeout' => 'Компьютер не ответил',
  'not_found' => 'Кнопки уже нет на деке',
  'launch_failed' => 'Компьютер не смог это запустить',
  'window_gone' => 'Это окно уже закрыто',
  'confirm_required' => 'Нужно подтверждение — обновите приложение',
  'unsupported' => 'Этот компьютер так не умеет',
  _ => 'Не получилось: $code',
};

/// The app glyph: a 3x2 deck of keys (same as the PC agent's icon).
class DeckGlyph extends StatelessWidget {
  const DeckGlyph({super.key, this.size = 72});
  final double size;

  @override
  Widget build(BuildContext context) => CustomPaint(size: Size.square(size), painter: _GlyphPainter());
}

class _GlyphPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final u = size.width / 64;
    RRect r(double x, double y, double w, double h, double rad) =>
        RRect.fromRectAndRadius(Rect.fromLTWH(x * u, y * u, w * u, h * u), Radius.circular(rad * u));
    canvas.drawRRect(r(2, 2, 60, 60, 14), Paint()..color = const Color(0xFF16181D));
    const keys = [
      (12.0, 14.0, C.accent),
      (26.5, 14.0, C.accentSoft),
      (41.0, 14.0, C.accent),
      (12.0, 29.0, C.accentSoft),
      (26.5, 29.0, C.accent),
      (41.0, 29.0, C.accentSoft),
    ];
    for (final (x, y, c) in keys) {
      canvas.drawRRect(r(x, y, 11, 11, 3), Paint()..color = c);
    }
    canvas.drawRRect(r(22, 47, 20, 4, 2), Paint()..color = const Color(0xFF5B616E));
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}

class StatusDot extends StatelessWidget {
  const StatusDot(this.status, {super.key, this.size = 9});
  final LinkStatus status;
  final double size;

  @override
  Widget build(BuildContext context) {
    final c = statusColor(status);
    return Container(
      width: size,
      height: size,
      decoration: BoxDecoration(
        color: c,
        shape: BoxShape.circle,
        boxShadow: status == LinkStatus.online ? [BoxShadow(color: c.withValues(alpha: .35), blurRadius: 6, spreadRadius: 1)] : null,
      ),
    );
  }
}

/// Small "swipe here" hint shown at the top/bottom edge of a page.
class SwipeHint extends StatelessWidget {
  const SwipeHint({super.key, required this.label, required this.up});
  final String label;
  final bool up;

  @override
  Widget build(BuildContext context) {
    final icon = Icon(up ? Icons.keyboard_arrow_up_rounded : Icons.keyboard_arrow_down_rounded, size: 18, color: C.muted);
    return Opacity(
      opacity: .7,
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          icon,
          const SizedBox(width: 4),
          Text(label, style: const TextStyle(color: C.muted, fontSize: 12)),
        ],
      ),
    );
  }
}
