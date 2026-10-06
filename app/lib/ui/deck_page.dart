import 'package:flutter/material.dart';

import '../protocol/layout.dart';
import '../protocol/link.dart';
import '../protocol/models.dart';
import '../state.dart';
import 'profile_picker.dart';
import 'theme.dart';
import 'tile.dart';

class DeckPage extends StatefulWidget {
  const DeckPage({
    super.key,
    required this.app,
    required this.link,
    required this.onShowMachines,
    required this.onAddMachine,
    this.half = false,
  });
  final AppState app;
  final MachineLink link;
  final VoidCallback onShowMachines;
  final VoidCallback onAddMachine;

  /// One of two computers side by side: no swipe hints or safe-area padding of its own
  /// (the home screen draws them once), and half the columns.
  final bool half;

  @override
  State<DeckPage> createState() => _DeckPageState();
}

class _DeckPageState extends State<DeckPage> {
  final _pages = PageController();
  int _page = 0;
  String? _shownKey; // machine/profile/folder currently on screen
  String? _folderId; // open folder of the shown profile

  @override
  void dispose() {
    _pages.dispose();
    super.dispose();
  }

  /// A different deck (another machine, profile or folder) starts on its first page.
  void _trackShown(String key) {
    if (_shownKey == key) return;
    final first = _shownKey == null;
    _shownKey = key;
    if (first) return;
    _page = 0;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted && _pages.hasClients) _pages.jumpToPage(0);
    });
  }

  @override
  Widget build(BuildContext context) {
    final link = widget.link;
    final st = link.state;
    final pinned = link.machine.pinnedProfile;
    if (st != null && pinned != null && st.profile(pinned) == null) {
      // The pinned profile was deleted on the PC: follow the PC again.
      WidgetsBinding.instance.addPostFrameCallback((_) => widget.app.setPinnedProfile(link.machine.id, null));
    }
    final shown = st?.shown(pinned);
    if (shown != null && _folderId != null && _openFolder(shown) == null) _folderId = null; // gone from the PC
    if (shown != null) _trackShown('${link.machine.id}/${shown.id}/${_folderId ?? ''}');
    final page = Column(
      children: [
        _Header(
          link: link,
          onTap: widget.onShowMachines,
          profileName: st != null && st.profiles.length > 1 ? shown!.name : null,
          pinned: pinned != null && st?.profile(pinned) != null,
          onProfileTap: () => showProfilePicker(context, widget.app, link),
        ),
        if (!widget.half) const SwipeHint(label: 'компьютеры', up: false),
        Expanded(child: _body(context, link)),
        if (!widget.half) const DeckFooter(),
      ],
    );
    // Back closes an open folder instead of leaving the deck.
    return PopScope(
      canPop: _folderId == null,
      onPopInvokedWithResult: (didPop, _) {
        if (!didPop && _folderId != null) setState(() => _folderId = null);
      },
      child: widget.half ? page : SafeArea(child: page),
    );
  }

  DeckButton? _openFolder(DeckProfile deck) {
    for (final b in deck.buttons) {
      if (b.isFolder && b.id == _folderId) return b;
    }
    return null;
  }

  GridLayout _grid(double width, double height, int columns) =>
      computeGrid(width, height, columns, gap: 12, landscape: MediaQuery.orientationOf(context) == Orientation.landscape);

  Widget _body(BuildContext context, MachineLink link) {
    final st = link.state;
    if (link.status == LinkStatus.unauthorized) {
      return Notice(
        icon: Icons.link_off_rounded,
        title: 'Компьютер отключил этот телефон',
        text: 'Нажмите «Подключить телефон» на компьютере и отсканируйте новый код.',
        action: FilledButton(onPressed: widget.onAddMachine, child: const Text('Подключить заново')),
      );
    }
    if (st == null) {
      return link.quiet
          ? Notice(icon: Icons.wifi_find_rounded, title: 'Подключаюсь к «${link.machine.name}»…', busy: true)
          : _offlineNotice(link);
    }
    final deck = st.shown(link.machine.pinnedProfile);
    if (deck.buttons.isEmpty) {
      return Notice(
        icon: Icons.dashboard_customize_outlined,
        title: st.profiles.length > 1 ? 'В профиле «${deck.name}» пока нет кнопок' : 'Дека пока пустая',
        text: 'Добавьте кнопки на компьютере: значок barphone в трее → «Открыть настройки».',
      );
    }
    // A dropped connection comes back in the background: the deck stays as it is and a press
    // waits for the reconnect. Only a computer that stays away gets dimmed, with a banner on
    // top (the grid itself does not move).
    final live = link.quiet;
    final folder = _openFolder(deck);
    // Inside a folder the first tile leads back.
    final items = <Object>[if (folder != null) _back, ...folder?.buttons ?? deck.buttons];
    return Stack(
      children: [
        Positioned.fill(
          // A short fade marks a profile switch (one PageView at a time: they share a controller).
          child: TweenAnimationBuilder<double>(
            key: ValueKey('${deck.id}/${folder?.id}'),
            tween: Tween(begin: 0, end: 1),
            duration: const Duration(milliseconds: 180),
            builder: (context, v, child) => Opacity(opacity: v, child: child),
            child: LayoutBuilder(
              builder: (context, box) {
                var g = _grid(box.maxWidth, box.maxHeight, deck.columns);
                var pages = g.pages(items.length);
                if (pages > 1) {
                  // Leave room for the page dots.
                  g = _grid(box.maxWidth, box.maxHeight - _dotsHeight, deck.columns);
                  pages = g.pages(items.length);
                }
                if (_page >= pages) _page = pages - 1;
                final view = PageView.builder(
                  key: ValueKey('${link.machine.id}/${deck.id}/${folder?.id}'),
                  controller: _pages,
                  itemCount: pages,
                  onPageChanged: (p) => setState(() => _page = p),
                  itemBuilder: (context, p) {
                    final slice = items.skip(p * g.perPage).take(g.perPage).toList();
                    return Center(
                      child: SizedBox(
                        width: g.columns * g.tile + (g.columns - 1) * g.gap,
                        child: Wrap(
                          spacing: g.gap,
                          runSpacing: g.gap,
                          children: [
                            for (final item in slice)
                              if (item is DeckButton)
                                DeckTile(
                                  key: ValueKey(item.id),
                                  button: item,
                                  link: link,
                                  size: g.tile,
                                  enabled: live || item.isFolder || item.isTimer || item.isTrackpad,
                                  onOpenFolder: (f) => setState(() => _folderId = f.id),
                                  timers: widget.app.timers,
                                )
                              else
                                BackTile(
                                  key: const ValueKey('back'),
                                  title: folder!.title,
                                  size: g.tile,
                                  onTap: () => setState(() => _folderId = null),
                                ),
                          ],
                        ),
                      ),
                    );
                  },
                );
                if (pages == 1) return view;
                return Column(
                  children: [
                    Expanded(child: view),
                    _dots(pages),
                  ],
                );
              },
            ),
          ),
        ),
        if (!live)
          Positioned(
            left: 0,
            right: 0,
            bottom: 0,
            child: _OfflineBanner(app: widget.app, link: link),
          ),
      ],
    );
  }

  static const _back = Object(); // the "back" tile of an open folder

  static const _dotsHeight = 18.0;

  Widget _dots(int pages) => SizedBox(
    height: _dotsHeight,
    child: Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        for (var i = 0; i < pages; i++)
          AnimatedContainer(
            duration: const Duration(milliseconds: 200),
            margin: const EdgeInsets.symmetric(horizontal: 3),
            width: i == _page ? 16 : 6,
            height: 6,
            decoration: BoxDecoration(color: i == _page ? C.accent : C.border, borderRadius: BorderRadius.circular(3)),
          ),
      ],
    ),
  );

  Widget _offlineNotice(MachineLink link) => Notice(
    icon: Icons.cloud_off_rounded,
    title: '«${link.machine.name}» не в сети',
    text: 'Компьютер выключен, спит или в другой сети. Проверьте, что barphone запущен на нём.',
    action: _WakeOrRetry(app: widget.app, link: link),
  );
}

/// "Swipe up for recent launches" under the deck.
class DeckFooter extends StatelessWidget {
  const DeckFooter({super.key});

  @override
  Widget build(BuildContext context) => const Padding(
    padding: EdgeInsets.only(top: 6, bottom: 8),
    child: SwipeHint(label: 'недавние', up: true),
  );
}

class _Header extends StatelessWidget {
  const _Header({required this.link, required this.onTap, required this.profileName, required this.pinned, required this.onProfileTap});
  final MachineLink link;
  final VoidCallback onTap;
  final String? profileName; // null: only one profile, no chip
  final bool pinned;
  final VoidCallback onProfileTap;

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(12),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 10, 12, 4),
        child: Row(
          children: [
            StatusDot(link.shownStatus),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                link.machine.name,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(fontSize: 17, fontWeight: FontWeight.w600),
              ),
            ),
            if (!link.quiet)
              Padding(
                padding: const EdgeInsets.only(right: 8),
                child: Text(statusText(link.shownStatus), style: TextStyle(color: statusColor(link.shownStatus), fontSize: 13)),
              ),
            if (profileName != null) _ProfileChip(name: profileName!, pinned: pinned, onTap: onProfileTap),
          ],
        ),
      ),
    );
  }
}

/// Which profile is on screen; tap to pin one or go back to following the PC.
class _ProfileChip extends StatelessWidget {
  const _ProfileChip({required this.name, required this.pinned, required this.onTap});
  final String name;
  final bool pinned;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: pinned ? C.accent.withValues(alpha: .18) : C.tile,
      shape: StadiumBorder(side: BorderSide(color: pinned ? C.accent : C.border)),
      child: InkWell(
        customBorder: const StadiumBorder(),
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 7),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(pinned ? Icons.push_pin_rounded : Icons.auto_awesome_rounded, size: 15, color: pinned ? C.accent : C.muted),
              const SizedBox(width: 6),
              ConstrainedBox(
                constraints: const BoxConstraints(maxWidth: 130),
                child: Text(
                  name,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600),
                ),
              ),
              const Icon(Icons.expand_more_rounded, size: 18, color: C.muted),
            ],
          ),
        ),
      ),
    );
  }
}

class _OfflineBanner extends StatelessWidget {
  const _OfflineBanner({required this.app, required this.link});
  final AppState app;
  final MachineLink link;

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: const EdgeInsets.fromLTRB(16, 4, 16, 8),
      padding: const EdgeInsets.fromLTRB(14, 8, 8, 8),
      decoration: BoxDecoration(
        color: C.surface,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: C.border),
      ),
      child: Row(
        children: [
          const Icon(Icons.cloud_off_rounded, size: 18, color: C.muted),
          const SizedBox(width: 10),
          Expanded(
            child: Text('Нет связи — показана последняя дека', style: const TextStyle(color: C.muted, fontSize: 13)),
          ),
          _WakeOrRetry(app: app, link: link, compact: true),
        ],
      ),
    );
  }
}

class _WakeOrRetry extends StatelessWidget {
  const _WakeOrRetry({required this.app, required this.link, this.compact = false});
  final AppState app;
  final MachineLink link;
  final bool compact;

  @override
  Widget build(BuildContext context) {
    if (link.machine.macs.isNotEmpty) {
      Future<void> onPressed() async {
        final sent = await app.wake(link.machine.id);
        if (context.mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: Text(sent ? 'Сигнал пробуждения отправлен' : 'Не удалось отправить сигнал'),
              duration: const Duration(seconds: 2),
            ),
          );
        }
      }

      return compact
          ? TextButton(onPressed: onPressed, child: const Text('Разбудить'))
          : FilledButton.icon(onPressed: onPressed, icon: const Icon(Icons.power_settings_new_rounded), label: const Text('Разбудить'));
    }
    return compact
        ? TextButton(onPressed: link.reconnectNow, child: const Text('Повторить'))
        : FilledButton(onPressed: link.reconnectNow, child: const Text('Повторить'));
  }
}

/// Centered icon + message + optional action, used for empty and error states.
class Notice extends StatelessWidget {
  const Notice({super.key, required this.icon, required this.title, this.text, this.action, this.busy = false});
  final IconData icon;
  final String title;
  final String? text;
  final Widget? action;
  final bool busy;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            busy
                ? const SizedBox.square(dimension: 40, child: CircularProgressIndicator(strokeWidth: 3, color: C.accent))
                : Icon(icon, size: 48, color: C.muted),
            const SizedBox(height: 18),
            Text(
              title,
              textAlign: TextAlign.center,
              style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w600),
            ),
            if (text != null) ...[
              const SizedBox(height: 8),
              Text(
                text!,
                textAlign: TextAlign.center,
                style: const TextStyle(color: C.muted, height: 1.35),
              ),
            ],
            if (action != null) ...[const SizedBox(height: 22), action!],
          ],
        ),
      ),
    );
  }
}
