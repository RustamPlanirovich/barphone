import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../protocol/link.dart';
import '../protocol/models.dart';
import '../timers.dart';
import 'glyphs.dart';
import 'theme.dart';
import 'trackpad.dart';
import 'volume_slider.dart';
import 'window_chooser.dart';

/// Icon of a deck button, fetched from the PC with the device token.
class ButtonIcon extends StatelessWidget {
  const ButtonIcon({super.key, required this.button, required this.link, required this.size});
  final DeckButton button;
  final MachineLink link;
  final double size;

  @override
  Widget build(BuildContext context) {
    final hash = button.icon;
    final url = hash == null ? null : link.iconUrl(hash);
    final glyph = button.glyph;
    final fallback = hasGlyph(glyph) ? GlyphIcon(glyph!, size: size) : _Letter(button: button, size: size);
    if (url == null) return fallback;
    return Image.network(
      url,
      headers: link.authHeaders,
      width: size,
      height: size,
      fit: BoxFit.contain,
      gaplessPlayback: true,
      filterQuality: FilterQuality.medium,
      errorBuilder: (_, _, _) => fallback,
      frameBuilder: (_, child, frame, sync) => sync || frame != null ? child : SizedBox.square(dimension: size),
    );
  }
}

class _Letter extends StatelessWidget {
  const _Letter({required this.button, required this.size});
  final DeckButton button;
  final double size;

  static const _palette = [
    Color(0xFF4F7CFF),
    Color(0xFF2EB8A6),
    Color(0xFFB36BFF),
    Color(0xFFFF7A45),
    Color(0xFFE85D8F),
    Color(0xFF5FA8FF),
  ];

  @override
  Widget build(BuildContext context) {
    final title = button.title.trim();
    final color = _palette[title.hashCode.abs() % _palette.length];
    final child = button.kind == 'url'
        ? Icon(Icons.language_rounded, size: size * .55, color: Colors.white)
        : Text(
            title.isEmpty ? '?' : title.characters.first.toUpperCase(),
            style: TextStyle(fontSize: size * .45, fontWeight: FontWeight.w700, color: Colors.white),
          );
    return Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      decoration: BoxDecoration(color: color.withValues(alpha: .85), borderRadius: BorderRadius.circular(size * .24)),
      child: child,
    );
  }
}

/// Feedback for a press: haptics on the phone, a flash on the tile, a message on failure.
/// When the app already has several windows open on the PC, the user picks one.
/// Buttons marked `confirm` (shutdown, restart) ask first.
Future<void> pressButton(BuildContext context, MachineLink link, DeckButton b, void Function(bool ok) flash) async {
  if (b.confirm && !await confirmPress(context, link, b)) return;
  HapticFeedback.lightImpact();
  final res = await link.launch(b.id, confirmed: b.confirm);
  if (res.needsChoice) {
    if (context.mounted) await showWindowChooser(context, link, b, res.windows, flash: flash);
    return;
  }
  flash(res.ok);
  if (!context.mounted) return;
  if (!res.ok) {
    showPressError(context, b, res.error);
  } else if (res.muted != null) {
    showNote(context, res.muted! ? 'Звук выключен' : 'Звук включён');
  } else if (res.value != null && b.isSlider) {
    showNote(context, '${b.title}: ${(res.value! * 100).round()}% — удерживайте и ведите пальцем');
  }
}

Future<bool> confirmPress(BuildContext context, MachineLink link, DeckButton b) async {
  HapticFeedback.mediumImpact();
  final ok = await showDialog<bool>(
    context: context,
    builder: (d) => AlertDialog(
      title: Text('${b.title}?'),
      content: Text('Компьютер «${link.machine.name}» выполнит это сразу.'),
      actions: [
        TextButton(onPressed: () => Navigator.pop(d, false), child: const Text('Отмена')),
        FilledButton(
          style: FilledButton.styleFrom(backgroundColor: C.danger, foregroundColor: Colors.white),
          onPressed: () => Navigator.pop(d, true),
          child: Text(b.title),
        ),
      ],
    ),
  );
  return ok == true;
}

void showNote(BuildContext context, String text) {
  ScaffoldMessenger.of(context)
    ..hideCurrentSnackBar()
    ..showSnackBar(SnackBar(content: Text(text), duration: const Duration(milliseconds: 1200)));
}

/// Timer button: start it, or (after asking) stop it.
Future<void> pressTimer(BuildContext context, DeckTimers timers, MachineLink link, DeckButton b) async {
  final key = DeckTimers.key(link.machine.id, b.id);
  final left = timers.left(key);
  if (left == null) {
    final s = b.seconds ?? 0;
    if (s <= 0) return;
    HapticFeedback.lightImpact();
    timers.start(key, Duration(seconds: s), b.title);
    return;
  }
  HapticFeedback.mediumImpact();
  final stop = await showDialog<bool>(
    context: context,
    builder: (d) => AlertDialog(
      title: Text('Остановить «${b.title}»?'),
      content: Text('Осталось ${formatLeft(left)}.'),
      actions: [
        TextButton(onPressed: () => Navigator.pop(d, false), child: const Text('Пусть идёт')),
        FilledButton(onPressed: () => Navigator.pop(d, true), child: const Text('Остановить')),
      ],
    ),
  );
  if (stop == true) timers.stop(key);
}

/// Long press: always offer the open windows and "start a new one".
Future<void> showButtonWindows(BuildContext context, MachineLink link, DeckButton b, void Function(bool ok) flash) async {
  HapticFeedback.mediumImpact();
  final res = await link.windows(b.id);
  if (!context.mounted) return;
  if (!res.ok) {
    showPressError(context, b, res.error);
    return;
  }
  await showWindowChooser(context, link, b, res.windows, flash: flash);
}

void showPressError(BuildContext context, DeckButton b, String? error) {
  HapticFeedback.heavyImpact();
  ScaffoldMessenger.of(context)
    ..hideCurrentSnackBar()
    ..showSnackBar(SnackBar(content: Text('${b.title}: ${launchError(error)}'), duration: const Duration(seconds: 2)));
}

/// First tile of an open folder: back to the profile's buttons.
class BackTile extends StatelessWidget {
  const BackTile({super.key, required this.title, required this.size, required this.onTap});
  final String title;
  final double size;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final s = size;
    return Semantics(
      button: true,
      label: 'Назад из папки $title',
      child: GestureDetector(
        onTap: () {
          HapticFeedback.selectionClick();
          onTap();
        },
        child: Container(
          width: s,
          height: s,
          padding: EdgeInsets.fromLTRB(s * .08, s * .1, s * .08, s * .07),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(s * .2),
            border: Border.all(color: C.border, width: 2),
          ),
          child: Column(
            children: [
              Expanded(
                child: Icon(Icons.arrow_back_rounded, size: s * .4, color: C.muted),
              ),
              SizedBox(height: s * .04),
              Text(
                title,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                textAlign: TextAlign.center,
                style: TextStyle(fontSize: (s * .11).clamp(10.0, 15.0), color: C.muted, height: 1.1),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class DeckTile extends StatefulWidget {
  const DeckTile({
    super.key,
    required this.button,
    required this.link,
    required this.size,
    required this.enabled,
    this.onOpenFolder,
    this.timers,
  });
  final DeckButton button;
  final MachineLink link;
  final double size;
  final bool enabled;
  final void Function(DeckButton folder)? onOpenFolder;
  final DeckTimers? timers; // needed by timer buttons

  @override
  State<DeckTile> createState() => _DeckTileState();
}

class _DeckTileState extends State<DeckTile> {
  bool _down = false;
  Color? _flash;
  Timer? _flashTimer;
  VolumeDrag? _drag;

  void _setFlash(bool ok) {
    if (!mounted) return;
    setState(() => _flash = ok ? C.ok : C.danger);
    _flashTimer?.cancel();
    _flashTimer = Timer(const Duration(milliseconds: 450), () {
      if (mounted) setState(() => _flash = null);
    });
  }

  @override
  void dispose() {
    _flashTimer?.cancel();
    _drag?.cancel(); // otherwise the volume panel would stay on screen
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = widget.size;
    final b = widget.button;
    final labelSize = (s * .11).clamp(10.0, 15.0);
    return Semantics(
      button: true,
      label: b.title,
      child: GestureDetector(
        onTapDown: widget.enabled ? (_) => setState(() => _down = true) : null,
        onTapCancel: () => setState(() => _down = false),
        onTapUp: (_) => setState(() => _down = false),
        onTap: !widget.enabled
            ? null
            : b.isFolder
            ? () {
                HapticFeedback.selectionClick();
                widget.onOpenFolder?.call(b);
              }
            : b.isTimer
            ? () {
                final timers = widget.timers;
                if (timers != null) pressTimer(context, timers, widget.link, b);
              }
            : b.isTrackpad
            ? () => openTrackpad(context, widget.link, b)
            : () => pressButton(context, widget.link, b, _setFlash),
        // Long press: app buttons offer their open windows, the volume button turns into a slider.
        onLongPress: widget.enabled && b.launchesApp ? () => showButtonWindows(context, widget.link, b, _setFlash) : null,
        onLongPressStart: widget.enabled && b.isSlider ? (_) => (_drag = VolumeDrag(context, widget.link, b)).start() : null,
        onLongPressMoveUpdate: b.isSlider ? (d) => _drag?.move(d.offsetFromOrigin) : null,
        onLongPressEnd: b.isSlider
            ? (_) {
                _drag?.end();
                _drag = null;
              }
            : null,
        child: AnimatedScale(
          scale: _down ? .92 : 1,
          duration: const Duration(milliseconds: 90),
          child: AnimatedContainer(
            duration: const Duration(milliseconds: 160),
            width: s,
            height: s,
            padding: EdgeInsets.fromLTRB(s * .08, s * .1, s * .08, s * .07),
            decoration: BoxDecoration(
              color: _down ? C.tilePressed : C.tile,
              borderRadius: BorderRadius.circular(s * .2),
              border: Border.all(color: _flash ?? Colors.transparent, width: 2.5),
              boxShadow: _flash == null ? null : [BoxShadow(color: _flash!.withValues(alpha: .35), blurRadius: 14)],
            ),
            child: Opacity(
              opacity: widget.enabled ? 1 : .4,
              child: Column(
                children: [
                  Expanded(
                    child: Center(
                      child: b.isTimer && widget.timers != null
                          ? _Countdown(button: b, link: widget.link, timers: widget.timers!, size: s)
                          : b.isStat
                          ? _LiveValue(button: b, link: widget.link, size: s)
                          : ButtonIcon(button: b, link: widget.link, size: s * .5),
                    ),
                  ),
                  SizedBox(height: s * .04),
                  Text(
                    b.title,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    textAlign: TextAlign.center,
                    style: TextStyle(fontSize: labelSize, color: const Color(0xFFC9CDD6), height: 1.1),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// A timer's face: its icon while idle, the time left (and a ring) while running.
class _Countdown extends StatelessWidget {
  const _Countdown({required this.button, required this.link, required this.timers, required this.size});
  final DeckButton button;
  final MachineLink link;
  final DeckTimers timers;
  final double size;

  @override
  Widget build(BuildContext context) {
    final key = DeckTimers.key(link.machine.id, button.id);
    return ListenableBuilder(
      listenable: timers,
      builder: (context, _) {
        final left = timers.left(key);
        if (left == null) return ButtonIcon(button: button, link: link, size: size * .5);
        final total = (button.seconds ?? 1) * 1000;
        return SizedBox.square(
          dimension: size * .58,
          child: Stack(
            fit: StackFit.expand,
            children: [
              CircularProgressIndicator(
                value: (left.inMilliseconds / total).clamp(0.0, 1.0),
                strokeWidth: size * .035,
                color: C.accent,
                backgroundColor: C.border,
              ),
              Center(
                child: Padding(
                  padding: EdgeInsets.all(size * .07),
                  child: FittedBox(
                    child: Text(
                      formatLeft(left),
                      style: const TextStyle(fontWeight: FontWeight.w700, fontFeatures: [FontFeature.tabularFigures()]),
                    ),
                  ),
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}

/// A live tile's face: the current value with a bar; its glyph until the first numbers.
class _LiveValue extends StatelessWidget {
  const _LiveValue({required this.button, required this.link, required this.size});
  final DeckButton button;
  final MachineLink link;
  final double size;

  @override
  Widget build(BuildContext context) {
    return ValueListenableBuilder<SysStats?>(
      valueListenable: link.stats,
      builder: (context, stats, _) {
        final v = stats?.of(button.stat!);
        if (v == null) return ButtonIcon(button: button, link: link, size: size * .5);
        final color = v > .9
            ? C.danger
            : v > .7
            ? C.warn
            : C.accent;
        final gb = button.stat == 'ram' && stats!.ramUsed != null && stats.ramTotal != null
            ? '${_gb(stats.ramUsed!)} / ${_gb(stats.ramTotal!)} ГБ'
            : null;
        return SizedBox(
          width: size * .7,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              SizedBox(
                height: size * .3,
                child: FittedBox(
                  child: Text(
                    '${(v * 100).round()}%',
                    style: const TextStyle(fontWeight: FontWeight.w700, fontFeatures: [FontFeature.tabularFigures()]),
                  ),
                ),
              ),
              SizedBox(height: size * .04),
              ClipRRect(
                borderRadius: BorderRadius.circular(3),
                child: LinearProgressIndicator(value: v, minHeight: size * .035, color: color, backgroundColor: C.border),
              ),
              if (gb != null) ...[
                SizedBox(height: size * .03),
                Text(
                  gb,
                  maxLines: 1,
                  style: TextStyle(fontSize: (size * .075).clamp(8.0, 12.0), color: C.muted),
                ),
              ],
            ],
          ),
        );
      },
    );
  }

  static String _gb(int bytes) => (bytes / (1 << 30)).toStringAsFixed(bytes >= 10 << 30 ? 0 : 1).replaceAll('.', ',');
}
