import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../protocol/link.dart';
import '../protocol/models.dart';
import 'theme.dart';
import 'tile.dart' show showPressError;

/// Hold a slider button (volume or brightness) and drag: right/up = more, left/down =
/// less. The PC follows the finger; a panel in the middle of the screen shows the level.
class VolumeDrag {
  VolumeDrag(this.context, this.link, this.button);

  final BuildContext context;
  final MachineLink link;
  final DeckButton button;

  static const _fullRange = 260.0; // logical px of travel for 0 → 100 %

  final _level = ValueNotifier<double?>(null); // null until the PC answered
  final _muted = ValueNotifier<bool>(false);
  OverlayEntry? _entry;
  double? _start;
  double? _pending;
  bool _sending = false;
  bool _ended = false;
  int _lastTick = -1;

  Future<void> start() async {
    HapticFeedback.mediumImpact();
    _entry = OverlayEntry(
      builder: (_) => _VolumePanel(title: button.title, brightness: button.glyph == 'brightness', level: _level, muted: _muted),
    );
    Overlay.of(context).insert(_entry!);
    final r = await link.volume(button.id);
    if (_ended) return;
    if (!r.ok || r.value == null) {
      _close();
      if (context.mounted) showPressError(context, button, r.error);
      return;
    }
    _start = r.value;
    _level.value = r.value;
    _muted.value = r.muted ?? false;
  }

  void move(Offset fromOrigin) {
    final start = _start;
    if (start == null) return;
    final v = (start + (fromOrigin.dx - fromOrigin.dy) / _fullRange).clamp(0.0, 1.0);
    _level.value = v;
    _muted.value = false;
    final tick = (v * 20).round(); // a click every 5 %
    if (tick != _lastTick) {
      _lastTick = tick;
      HapticFeedback.selectionClick();
    }
    _pending = v;
    _flush();
  }

  /// Sends the latest level; at most one request in flight, so a fast finger never
  /// queues up stale values.
  Future<void> _flush() async {
    if (_sending || _pending == null) return;
    _sending = true;
    final v = _pending!;
    _pending = null;
    await link.volume(button.id, v);
    await Future<void>.delayed(const Duration(milliseconds: 30));
    _sending = false;
    if (_pending != null) unawaited(_flush());
  }

  void end() {
    _ended = true;
    Timer(const Duration(milliseconds: 500), _close); // let the final level be seen
  }

  /// The tile went away mid-drag (e.g. the profile switched): drop the panel now.
  void cancel() {
    _ended = true;
    _close();
  }

  void _close() {
    _entry?.remove();
    _entry = null;
  }
}

class _VolumePanel extends StatelessWidget {
  const _VolumePanel({required this.title, required this.brightness, required this.level, required this.muted});
  final String title;
  final bool brightness;
  final ValueNotifier<double?> level;
  final ValueNotifier<bool> muted;

  @override
  Widget build(BuildContext context) {
    return IgnorePointer(
      child: Center(
        child: Material(
          color: C.surface.withValues(alpha: .96),
          elevation: 12,
          borderRadius: BorderRadius.circular(24),
          child: SizedBox(
            width: 250,
            child: Padding(
              padding: const EdgeInsets.fromLTRB(24, 22, 24, 20),
              child: ListenableBuilder(
                listenable: Listenable.merge([level, muted]),
                builder: (context, _) {
                  final v = level.value;
                  final icon = brightness
                      ? (v ?? 0) < .5
                            ? Icons.brightness_low_rounded
                            : Icons.brightness_high_rounded
                      : muted.value || v == 0
                      ? Icons.volume_off_rounded
                      : (v ?? 0) < .5
                      ? Icons.volume_down_rounded
                      : Icons.volume_up_rounded;
                  return Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Icon(icon, size: 40, color: C.accent),
                      const SizedBox(height: 8),
                      Text(v == null ? '…' : '${(v * 100).round()}%', style: const TextStyle(fontSize: 34, fontWeight: FontWeight.w700)),
                      const SizedBox(height: 12),
                      ClipRRect(
                        borderRadius: BorderRadius.circular(6),
                        child: LinearProgressIndicator(value: v, minHeight: 10, color: C.accent, backgroundColor: C.tile),
                      ),
                      const SizedBox(height: 12),
                      Text(
                        brightness ? '$title · ← темнее   ярче →' : '$title · ← тише   громче →',
                        style: const TextStyle(color: C.muted, fontSize: 12),
                      ),
                    ],
                  );
                },
              ),
            ),
          ),
        ),
      ),
    );
  }
}
