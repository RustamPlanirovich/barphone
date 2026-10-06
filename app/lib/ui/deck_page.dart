import 'package:flutter/material.dart';

import '../protocol/layout.dart';
import '../protocol/link.dart';
import '../state.dart';
import 'profile_picker.dart';
import 'theme.dart';
import 'tile.dart';

class DeckPage extends StatefulWidget {
  const DeckPage({super.key, required this.app, required this.link, required this.onShowMachines, required this.onAddMachine});
  final AppState app;
  final MachineLink link;
  final VoidCallback onShowMachines;
  final VoidCallback onAddMachine;

  @override
  State<DeckPage> createState() => _DeckPageState();
}

class _DeckPageState extends State<DeckPage> {
  final _pages = PageController();
  int _page = 0;
  String? _shownKey; // machine/profile currently on screen

  @override
  void dispose() {
    _pages.dispose();
    super.dispose();
  }

  /// A different deck (another machine or profile) starts on its first page.
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
    if (shown != null) _trackShown('${link.machine.id}/${shown.id}');
    return SafeArea(
      child: Column(
        children: [
          _Header(
            link: link,
            onTap: widget.onShowMachines,
            profileName: st != null && st.profiles.length > 1 ? shown!.name : null,
            pinned: pinned != null && st?.profile(pinned) != null,
            onProfileTap: () => showProfilePicker(context, widget.app, link),
          ),
          const SwipeHint(label: 'компьютеры', up: false),
          Expanded(child: _body(context, link)),
          _footer(link),
        ],
      ),
    );
  }

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
      return link.status == LinkStatus.connecting
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
    final online = link.status == LinkStatus.online;
    return Column(
      children: [
        if (!online) _OfflineBanner(app: widget.app, link: link),
        Expanded(
          // A short fade marks a profile switch (one PageView at a time: they share a controller).
          child: TweenAnimationBuilder<double>(
            key: ValueKey(deck.id),
            tween: Tween(begin: 0, end: 1),
            duration: const Duration(milliseconds: 180),
            builder: (context, v, child) => Opacity(opacity: v, child: child),
            child: LayoutBuilder(
              builder: (context, box) {
                var g = computeGrid(box.maxWidth, box.maxHeight, deck.columns, gap: 12);
                var pages = g.pages(deck.buttons.length);
                if (pages > 1) {
                  // Leave room for the page dots.
                  g = computeGrid(box.maxWidth, box.maxHeight - _dotsHeight, deck.columns, gap: 12);
                  pages = g.pages(deck.buttons.length);
                }
                if (_page >= pages) _page = pages - 1;
                final view = PageView.builder(
                  key: ValueKey('${link.machine.id}/${deck.id}'),
                  controller: _pages,
                  itemCount: pages,
                  onPageChanged: (p) => setState(() => _page = p),
                  itemBuilder: (context, p) {
                    final slice = deck.buttons.skip(p * g.perPage).take(g.perPage).toList();
                    return Center(
                      child: SizedBox(
                        width: g.columns * g.tile + (g.columns - 1) * g.gap,
                        child: Wrap(
                          spacing: g.gap,
                          runSpacing: g.gap,
                          children: [
                            for (final b in slice) DeckTile(key: ValueKey(b.id), button: b, link: link, size: g.tile, enabled: online),
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
      ],
    );
  }

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

  Widget _footer(MachineLink link) => const Padding(
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
            StatusDot(link.status),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                link.machine.name,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(fontSize: 17, fontWeight: FontWeight.w600),
              ),
            ),
            if (link.status != LinkStatus.online)
              Padding(
                padding: const EdgeInsets.only(right: 8),
                child: Text(statusText(link.status), style: TextStyle(color: statusColor(link.status), fontSize: 13)),
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
      margin: const EdgeInsets.fromLTRB(16, 8, 16, 4),
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
            child: Text(
              link.status == LinkStatus.connecting ? 'Подключение…' : 'Нет связи — показана последняя дека',
              style: const TextStyle(color: C.muted, fontSize: 13),
            ),
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
