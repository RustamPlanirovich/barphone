// Deck timers (Pomodoro and the like): they count down on the phone, the PC is not involved.
import 'dart:async';

import 'package:flutter/foundation.dart';

/// A running timer: when it ends (wall clock, so it survives rebuilds, the split screen and
/// the app going to the background) and what to call it when it does.
typedef TimerRun = ({DateTime end, String title});

class DeckTimers extends ChangeNotifier {
  DeckTimers({DateTime Function()? now}) : now = now ?? DateTime.now;

  /// The clock; tests replace it.
  final DateTime Function() now;

  final _runs = <String, TimerRun>{};
  final _finished = StreamController<String>.broadcast();
  Timer? _tick;

  /// Titles of timers that just ran out.
  Stream<String> get finished => _finished.stream;

  /// Key of a timer button: buttons of different computers may share an ID.
  static String key(String machineId, String buttonId) => '$machineId/$buttonId';

  bool running(String key) => _runs.containsKey(key);

  /// Time left, or null when the timer is not running.
  Duration? left(String key) {
    final run = _runs[key];
    if (run == null) return null;
    final d = run.end.difference(now());
    return d.isNegative ? Duration.zero : d;
  }

  void start(String key, Duration length, String title) {
    _runs[key] = (end: now().add(length), title: title);
    _tick ??= Timer.periodic(const Duration(milliseconds: 250), (_) => check());
    notifyListeners();
  }

  void stop(String key) {
    if (_runs.remove(key) == null) return;
    _idle();
    notifyListeners();
  }

  /// Ends timers whose time is up; runs on every tick (and after the app comes back).
  void check() {
    final t = now();
    final done = [
      for (final e in _runs.entries)
        if (!e.value.end.isAfter(t)) e,
    ];
    for (final e in done) {
      _runs.remove(e.key);
      _finished.add(e.value.title);
    }
    _idle();
    notifyListeners(); // the countdowns on the tiles
  }

  void _idle() {
    if (_runs.isEmpty) {
      _tick?.cancel();
      _tick = null;
    }
  }

  @override
  void dispose() {
    _tick?.cancel();
    _finished.close();
    super.dispose();
  }
}

/// "24:59", "1:05:00".
String formatLeft(Duration d) {
  final s = (d.inMilliseconds / 1000).ceil();
  final h = s ~/ 3600, m = s % 3600 ~/ 60, sec = s % 60;
  final mm = h > 0 ? m.toString().padLeft(2, '0') : '$m';
  return '${h > 0 ? '$h:' : ''}$mm:${sec.toString().padLeft(2, '0')}';
}
