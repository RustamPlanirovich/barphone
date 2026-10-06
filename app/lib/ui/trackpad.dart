import 'dart:async';
import 'dart:math';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../protocol/link.dart';
import '../protocol/models.dart';
import 'theme.dart';

Future<void> openTrackpad(BuildContext context, MachineLink link, DeckButton b) {
  HapticFeedback.selectionClick();
  return Navigator.of(context).push(
    MaterialPageRoute<void>(
      builder: (_) => TrackpadScreen(link: link, button: b),
    ),
  );
}

/// The phone as the PC's touchpad and keyboard: one finger moves the cursor, a tap
/// clicks, two fingers scroll, a two-finger tap is a right click. Typed text goes to the
/// window in front on the PC.
class TrackpadScreen extends StatefulWidget {
  const TrackpadScreen({super.key, required this.link, required this.button});
  final MachineLink link;
  final DeckButton button;

  @override
  State<TrackpadScreen> createState() => _TrackpadScreenState();
}

class _TrackpadScreenState extends State<TrackpadScreen> {
  static const _tapTime = Duration(milliseconds: 220);
  static const _tapSlop = 10.0; // logical px a tap may wander
  static const _wheelPerPx = 4.0; // 30 px of two-finger travel = one wheel notch (120)
  // The keyboard field always holds this invisible character, so a backspace on an
  // "empty" field is still noticed.
  static const _sentinel = '\u200B'; // zero-width space

  final _touches = <int, Offset>{};
  Offset _move = Offset.zero; // not yet sent, fractions of a pixel included
  double _wheel = 0;
  Timer? _flushTimer;
  DateTime? _downAt;
  int _fingers = 0; // most fingers down during the current touch
  double _travel = 0;
  bool _warned = false;

  final _text = TextEditingController(text: _sentinel);
  final _focus = FocusNode();
  bool _typing = false;

  String get _id => widget.button.id;
  MachineLink get _link => widget.link;

  @override
  void initState() {
    super.initState();
    _text.addListener(_onText);
    _focus.addListener(() => setState(() => _typing = _focus.hasFocus));
  }

  @override
  void dispose() {
    _flushTimer?.cancel();
    _text.dispose();
    _focus.dispose();
    super.dispose();
  }

  // ---- touch ----

  void _down(PointerDownEvent e) {
    if (_touches.isEmpty) {
      _downAt = DateTime.now();
      _fingers = 0;
      _travel = 0;
    }
    _touches[e.pointer] = e.localPosition;
    _fingers = max(_fingers, _touches.length);
  }

  void _moved(PointerMoveEvent e) {
    final prev = _touches[e.pointer];
    if (prev == null) return;
    _touches[e.pointer] = e.localPosition;
    final d = e.localPosition - prev;
    _travel += d.distance;
    if (_touches.length == 1 && _fingers == 1) {
      // Slow = precise, fast = far: the gain grows with the speed of the finger.
      _move += d * (1.6 + min(d.distance, 40) / 12);
    } else if (_touches.length >= 2) {
      _wheel += -d.dy * _wheelPerPx / _touches.length; // natural scrolling, averaged over fingers
    }
    _flushTimer ??= Timer(const Duration(milliseconds: 16), _flush);
  }

  void _up(PointerEvent e) {
    _touches.remove(e.pointer);
    if (_touches.isNotEmpty) return;
    final quick = _downAt != null && DateTime.now().difference(_downAt!) < _tapTime;
    if (quick && _travel < _tapSlop) _click(_fingers >= 2 ? 'right' : 'left');
  }

  /// At most one move and one wheel message per frame.
  void _flush() {
    _flushTimer = null;
    final dx = _move.dx.truncate(), dy = _move.dy.truncate();
    if (dx != 0 || dy != 0) {
      _link.pointer(_id, dx, dy);
      _move -= Offset(dx.toDouble(), dy.toDouble());
    }
    final w = _wheel.truncate();
    if (w != 0) {
      _link.scroll(_id, 0, w);
      _wheel -= w;
    }
  }

  Future<void> _click(String button) async {
    HapticFeedback.selectionClick();
    _check(await _link.click(_id, button));
  }

  /// Says once per screen why input does not arrive (no connection, no mouse on a Mac).
  void _check(LaunchResult r) {
    if (r.ok || _warned || !mounted) return;
    _warned = true;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(launchError(r.error)), duration: const Duration(seconds: 3)));
  }

  // ---- keyboard ----

  void _onText() {
    final v = _text.value;
    if (v.composing.isValid && !v.composing.isCollapsed) return; // the keyboard is still composing a word
    final t = v.text;
    if (t == _sentinel) return;
    if (!t.startsWith(_sentinel)) {
      _key('Backspace');
    } else {
      final typed = t.substring(_sentinel.length);
      if (typed.isNotEmpty) _link.typeText(_id, typed).then(_check);
    }
    _text.value = const TextEditingValue(
      text: _sentinel,
      selection: TextSelection.collapsed(offset: _sentinel.length),
    );
  }

  void _key(String key) {
    HapticFeedback.selectionClick();
    _link.key(_id, key).then(_check);
  }

  void _toggleKeyboard() {
    if (_focus.hasFocus) {
      _focus.unfocus();
    } else {
      _focus.requestFocus();
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(4, 4, 8, 4),
              child: Row(
                children: [
                  IconButton(onPressed: () => Navigator.pop(context), icon: const Icon(Icons.arrow_back_rounded)),
                  Expanded(
                    child: Text(
                      '${widget.button.title} · ${_link.machine.name}',
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(fontSize: 17, fontWeight: FontWeight.w600),
                    ),
                  ),
                  IconButton.filledTonal(
                    tooltip: 'Клавиатура',
                    isSelected: _typing,
                    onPressed: _toggleKeyboard,
                    icon: const Icon(Icons.keyboard_rounded),
                  ),
                ],
              ),
            ),
            Expanded(
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 12),
                child: Listener(
                  key: const ValueKey('pad'),
                  behavior: HitTestBehavior.opaque,
                  onPointerDown: _down,
                  onPointerMove: _moved,
                  onPointerUp: _up,
                  onPointerCancel: _up,
                  child: Container(
                    decoration: BoxDecoration(
                      color: C.tile,
                      borderRadius: BorderRadius.circular(24),
                      border: Border.all(color: C.border),
                    ),
                    alignment: Alignment.center,
                    child: const Padding(
                      padding: EdgeInsets.all(24),
                      child: Text(
                        'Водите пальцем — курсор\nТап — щелчок · два пальца — прокрутка\nТап двумя пальцами — правый щелчок',
                        textAlign: TextAlign.center,
                        style: TextStyle(color: C.muted, height: 1.6),
                      ),
                    ),
                  ),
                ),
              ),
            ),
            if (_typing) _keysRow(),
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 12),
              child: Row(
                children: [
                  Expanded(
                    child: FilledButton.tonal(onPressed: () => _click('left'), child: const Text('Левая')),
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: FilledButton.tonal(onPressed: () => _click('right'), child: const Text('Правая')),
                  ),
                ],
              ),
            ),
            // Invisible: it only brings up the phone's keyboard and catches what is typed.
            SizedBox(
              height: 1,
              child: Opacity(
                opacity: 0,
                child: TextField(
                  key: const ValueKey('keyboard'),
                  controller: _text,
                  focusNode: _focus,
                  autocorrect: false,
                  enableSuggestions: false,
                  enableIMEPersonalizedLearning: false,
                  textInputAction: TextInputAction.send,
                  onSubmitted: (_) => _key('Enter'),
                  onEditingComplete: () {}, // keep the keyboard open after Enter
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _keysRow() {
    const keys = [
      ('Esc', 'Escape'),
      ('Tab', 'Tab'),
      ('←', 'Left'),
      ('↑', 'Up'),
      ('↓', 'Down'),
      ('→', 'Right'),
      ('⌫', 'Backspace'),
      ('⏎', 'Enter'),
    ];
    return SizedBox(
      height: 48,
      child: ListView(
        scrollDirection: Axis.horizontal,
        padding: const EdgeInsets.fromLTRB(12, 8, 12, 0),
        children: [
          for (final (label, key) in keys)
            Padding(
              padding: const EdgeInsets.only(right: 6),
              child: OutlinedButton(
                style: OutlinedButton.styleFrom(minimumSize: const Size(48, 40), padding: const EdgeInsets.symmetric(horizontal: 12)),
                onPressed: () => _key(key),
                child: Text(label),
              ),
            ),
        ],
      ),
    );
  }
}
