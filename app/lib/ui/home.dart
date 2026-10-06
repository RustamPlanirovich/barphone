import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../protocol/link.dart';
import '../protocol/models.dart';
import '../state.dart';
import 'add_machine.dart';
import 'call_panel.dart';
import 'deck_page.dart';
import 'machines_page.dart';
import 'recent_page.dart';
import 'theme.dart';

/// Three vertically stacked pages: swipe down from the deck for computers,
/// swipe up for recent launches.
class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key, required this.app});
  final AppState app;

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  static const _machinesPage = 0, _deckPage = 1;
  final _pages = PageController(initialPage: _deckPage);
  int _page = _deckPage;
  StreamSubscription<String>? _timeUp;
  StreamSubscription<AgentNotice>? _noticeSub;
  OverlayEntry? _noticeEntry;
  Timer? _noticeTimer;

  @override
  void initState() {
    super.initState();
    _timeUp = widget.app.timers.finished.listen(_onTimeUp);
    _noticeSub = widget.app.notices.listen(_showNotice);
  }

  @override
  void dispose() {
    _timeUp?.cancel();
    _noticeSub?.cancel();
    _hideNotice();
    _pages.dispose();
    super.dispose();
  }

  /// A timer ran out: buzz a few times and say so until dismissed.
  Future<void> _onTimeUp(String title) async {
    if (!mounted) return;
    unawaited(_buzz());
    await showDialog<void>(
      context: context,
      builder: (d) => AlertDialog(
        icon: const Icon(Icons.alarm_rounded, size: 40, color: C.accent),
        title: Text(title),
        content: const Text('Время вышло'),
        actions: [FilledButton(onPressed: () => Navigator.pop(d), child: const Text('OK'))],
      ),
    );
  }

  /// A notice from a computer: a card over the top of the screen (the deck does not move),
  /// gone after a few seconds or on a tap; a newer one replaces it.
  void _showNotice(AgentNotice n) {
    if (!mounted) return;
    _hideNotice();
    n.level == 'error' ? HapticFeedback.heavyImpact() : HapticFeedback.mediumImpact();
    _noticeEntry = OverlayEntry(
      builder: (_) => NoticeCard(notice: n, onTap: _hideNotice),
    );
    Overlay.of(context).insert(_noticeEntry!);
    _noticeTimer = Timer(Duration(seconds: n.level == 'error' ? 10 : 6), _hideNotice);
  }

  void _hideNotice() {
    _noticeTimer?.cancel();
    _noticeTimer = null;
    _noticeEntry?.remove();
    _noticeEntry = null;
  }

  static Future<void> _buzz() async {
    for (var i = 0; i < 4; i++) {
      HapticFeedback.heavyImpact();
      await Future<void>.delayed(const Duration(milliseconds: 300));
    }
  }

  void _go(int page) {
    if (_pages.hasClients) _pages.animateToPage(page, duration: const Duration(milliseconds: 320), curve: Curves.easeOutCubic);
  }

  // The side pages contain vertical lists that would swallow the swipe back to the deck.
  // Once such a list is pinned against its edge, keep pulling = go back to the deck.
  double _overscroll = 0;

  Widget _handBackAtEdge({required bool towardsBottom, required Widget child}) {
    return NotificationListener<ScrollNotification>(
      onNotification: (n) {
        if (n.metrics.axis != Axis.vertical) return false;
        if (n is ScrollStartNotification) _overscroll = 0;
        if (n is OverscrollNotification && n.dragDetails != null) {
          _overscroll += n.overscroll;
          if (towardsBottom ? _overscroll > 36 : _overscroll < -36) {
            _overscroll = 0;
            _go(_deckPage);
          }
        }
        return false;
      },
      child: child,
    );
  }

  void _openAdd() {
    Navigator.of(context).push(MaterialPageRoute<void>(builder: (_) => AddMachineScreen(app: widget.app))).then((_) => _go(_deckPage));
  }

  /// The deck page. A landscape screen with a second computer connected shows both decks
  /// side by side: the active one on the left.
  Widget _deck(BuildContext context, MachineLink active) {
    final landscape = MediaQuery.orientationOf(context) == Orientation.landscape;
    final other = landscape ? widget.app.companion : null;
    Widget half(MachineLink link) => Expanded(
      child: DeckPage(app: widget.app, link: link, half: true, onShowMachines: () => _go(_machinesPage), onAddMachine: _openAdd),
    );
    if (other == null) {
      final single = DeckPage(app: widget.app, link: active, onShowMachines: () => _go(_machinesPage), onAddMachine: _openAdd);
      if (!landscape) return single;
      // One computer and a Meet call on it: its controls take the second half.
      return ValueListenableBuilder<CallInfo?>(
        valueListenable: active.call,
        builder: (context, call, _) =>
            call == null || !active.present ? single : _split(half(active), Expanded(child: CallPanel(link: active))),
      );
    }
    return _split(half(active), half(other));
  }

  /// Two halves side by side with the swipe hints shared.
  Widget _split(Widget left, Widget right) {
    return SafeArea(
      child: Column(
        children: [
          const SwipeHint(label: 'компьютеры', up: false),
          Expanded(
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                left,
                Container(width: 1, margin: const EdgeInsets.symmetric(vertical: 12), color: C.border),
                right,
              ],
            ),
          ),
          const DeckFooter(),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: widget.app,
      builder: (context, _) {
        final active = widget.app.active;
        if (active == null) return _Welcome(onAdd: _openAdd);
        return PopScope(
          canPop: _page == _deckPage,
          onPopInvokedWithResult: (didPop, _) {
            if (!didPop) _go(_deckPage);
          },
          child: Scaffold(
            body: PageView(
              controller: _pages,
              scrollDirection: Axis.vertical,
              onPageChanged: (p) => setState(() => _page = p),
              children: [
                _handBackAtEdge(
                  towardsBottom: true,
                  child: MachinesPage(app: widget.app, onPicked: () => _go(_deckPage), onAdd: _openAdd),
                ),
                _deck(context, active),
                _handBackAtEdge(towardsBottom: false, child: RecentPage(link: active)),
              ],
            ),
          ),
        );
      },
    );
  }
}

class _Welcome extends StatelessWidget {
  const _Welcome({required this.onAdd});
  final VoidCallback onAdd;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 32),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                const DeckGlyph(size: 96),
                const SizedBox(height: 24),
                const Text('barphone', style: TextStyle(fontSize: 30, fontWeight: FontWeight.w800, letterSpacing: .3)),
                const SizedBox(height: 12),
                const Text(
                  'Дека для компьютера: нажмите кнопку на телефоне — на компьютере откроется программа.',
                  textAlign: TextAlign.center,
                  style: TextStyle(color: C.muted, fontSize: 15, height: 1.4),
                ),
                const SizedBox(height: 28),
                const _Step(n: 1, text: 'Запустите barphone-agent на компьютере'),
                const _Step(n: 2, text: 'В настройках нажмите «Подключить телефон»'),
                const _Step(n: 3, text: 'Отсканируйте QR-код этим телефоном'),
                const SizedBox(height: 28),
                FilledButton.icon(
                  onPressed: onAdd,
                  icon: const Icon(Icons.qr_code_scanner_rounded),
                  label: const Text('Добавить компьютер'),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _Step extends StatelessWidget {
  const _Step({required this.n, required this.text});
  final int n;
  final String text;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 5),
      child: Row(
        children: [
          Container(
            width: 26,
            height: 26,
            alignment: Alignment.center,
            decoration: const BoxDecoration(color: C.tile, shape: BoxShape.circle),
            child: Text(
              '$n',
              style: const TextStyle(fontWeight: FontWeight.w700, color: C.accent),
            ),
          ),
          const SizedBox(width: 12),
          Expanded(child: Text(text)),
        ],
      ),
    );
  }
}

class NoticeCard extends StatelessWidget {
  const NoticeCard({super.key, required this.notice, required this.onTap});
  final AgentNotice notice;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final (icon, color) = switch (notice.level) {
      'ok' => (Icons.check_circle_rounded, C.ok),
      'error' => (Icons.error_rounded, C.danger),
      _ => (Icons.notifications_rounded, C.accent),
    };
    final title = [notice.machine, if (notice.title.isNotEmpty) notice.title].join(' · ');
    return Align(
      alignment: Alignment.topCenter,
      child: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(12, 8, 12, 0),
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 560),
            child: Material(
              color: C.surface,
              elevation: 10,
              borderRadius: BorderRadius.circular(16),
              child: InkWell(
                borderRadius: BorderRadius.circular(16),
                onTap: onTap,
                child: Container(
                  padding: const EdgeInsets.fromLTRB(14, 12, 16, 12),
                  decoration: BoxDecoration(
                    borderRadius: BorderRadius.circular(16),
                    border: Border.all(color: color.withValues(alpha: .6)),
                  ),
                  child: Row(
                    children: [
                      Icon(icon, color: color, size: 26),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Text(
                              title,
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                              style: const TextStyle(fontWeight: FontWeight.w600),
                            ),
                            if (notice.text.isNotEmpty)
                              Text(
                                notice.text,
                                maxLines: 3,
                                overflow: TextOverflow.ellipsis,
                                style: const TextStyle(color: C.muted),
                              ),
                          ],
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
