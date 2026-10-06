import 'dart:async';

import 'package:flutter/material.dart';

import '../protocol/link.dart';
import '../protocol/models.dart';
import 'deck_page.dart' show Notice;
import 'theme.dart';
import 'tile.dart';

String ago(DateTime t) {
  final d = DateTime.now().difference(t);
  if (d.inSeconds < 45) return 'только что';
  if (d.inMinutes < 60) return '${d.inMinutes} мин назад';
  if (d.inHours < 24) return '${d.inHours} ч назад';
  if (d.inDays < 7) return '${d.inDays} дн назад';
  return '${t.day.toString().padLeft(2, '0')}.${t.month.toString().padLeft(2, '0')}';
}

class RecentPage extends StatelessWidget {
  const RecentPage({super.key, required this.link});
  final MachineLink link;

  @override
  Widget build(BuildContext context) {
    final items = link.state?.recentButtons ?? const [];
    final online = link.status == LinkStatus.online;
    return SafeArea(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Center(
            child: Padding(
              padding: EdgeInsets.only(top: 8),
              child: SwipeHint(label: 'к деке', up: false),
            ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 12, 20, 4),
            child: Text('Недавние', style: Theme.of(context).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w700)),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 0, 20, 12),
            child: Text('Запущено с деки на «${link.machine.name}»', style: const TextStyle(color: C.muted)),
          ),
          Expanded(
            child: items.isEmpty
                ? const Notice(icon: Icons.history_rounded, title: 'Пока пусто', text: 'Здесь появится то, что вы запускали с деки.')
                : ListView.separated(
                    padding: const EdgeInsets.fromLTRB(16, 0, 16, 24),
                    itemCount: items.length,
                    separatorBuilder: (_, _) => const SizedBox(height: 8),
                    itemBuilder: (context, i) => _RecentRow(link: link, button: items[i].$1, at: items[i].$2, enabled: online),
                  ),
          ),
        ],
      ),
    );
  }
}

class _RecentRow extends StatefulWidget {
  const _RecentRow({required this.link, required this.button, required this.at, required this.enabled});
  final MachineLink link;
  final DeckButton button;
  final DateTime at;
  final bool enabled;

  @override
  State<_RecentRow> createState() => _RecentRowState();
}

class _RecentRowState extends State<_RecentRow> {
  Color? _flash;
  Timer? _t;

  void _setFlash(bool ok) {
    if (!mounted) return;
    setState(() => _flash = ok ? C.ok : C.danger);
    _t?.cancel();
    _t = Timer(const Duration(milliseconds: 450), () => mounted ? setState(() => _flash = null) : null);
  }

  @override
  void dispose() {
    _t?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Material(
      color: C.tile,
      borderRadius: BorderRadius.circular(16),
      child: InkWell(
        borderRadius: BorderRadius.circular(16),
        onTap: widget.enabled ? () => pressButton(context, widget.link, widget.button, _setFlash) : null,
        onLongPress: widget.enabled && widget.button.launchesApp
            ? () => showButtonWindows(context, widget.link, widget.button, _setFlash)
            : null,
        child: AnimatedContainer(
          duration: const Duration(milliseconds: 160),
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(16),
            border: Border.all(color: _flash ?? Colors.transparent, width: 2),
          ),
          child: Opacity(
            opacity: widget.enabled ? 1 : .45,
            child: Row(
              children: [
                ButtonIcon(button: widget.button, link: widget.link, size: 40),
                const SizedBox(width: 14),
                Expanded(
                  child: Text(widget.button.title, maxLines: 1, overflow: TextOverflow.ellipsis, style: const TextStyle(fontSize: 16)),
                ),
                Text(ago(widget.at), style: const TextStyle(color: C.muted, fontSize: 13)),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
