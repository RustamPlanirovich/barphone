import 'package:flutter/material.dart';

import '../protocol/link.dart';
import '../state.dart';
import 'theme.dart';

class MachinesPage extends StatelessWidget {
  const MachinesPage({super.key, required this.app, required this.onPicked, required this.onAdd});
  final AppState app;
  final VoidCallback onPicked;
  final VoidCallback onAdd;

  @override
  Widget build(BuildContext context) {
    final machines = app.machines;
    final activeId = app.active?.machine.id;
    return SafeArea(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 20, 20, 4),
            child: Text('Компьютеры', style: Theme.of(context).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w700)),
          ),
          const Padding(
            padding: EdgeInsets.fromLTRB(20, 0, 20, 12),
            child: Text('Нажмите, чтобы переключить деку. Удерживайте — действия.', style: TextStyle(color: C.muted)),
          ),
          Expanded(
            child: ListView(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 16),
              children: [
                for (final l in machines)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 10),
                    child: _MachineCard(
                      link: l,
                      active: l.machine.id == activeId,
                      onTap: () {
                        app.setActive(l.machine.id);
                        onPicked();
                      },
                      onLongPress: () => _actions(context, l),
                    ),
                  ),
                const SizedBox(height: 6),
                OutlinedButton.icon(
                  onPressed: onAdd,
                  icon: const Icon(Icons.add_rounded),
                  label: const Text('Добавить компьютер'),
                  style: OutlinedButton.styleFrom(
                    padding: const EdgeInsets.symmetric(vertical: 16),
                    side: const BorderSide(color: C.border),
                    foregroundColor: C.text,
                    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
                  ),
                ),
              ],
            ),
          ),
          const Padding(
            padding: EdgeInsets.only(bottom: 10),
            child: Center(child: SwipeHint(label: 'к деке', up: true)),
          ),
        ],
      ),
    );
  }

  void _actions(BuildContext context, MachineLink l) {
    showModalBottomSheet<void>(
      context: context,
      showDragHandle: true,
      builder: (sheet) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            ListTile(
              title: Text(l.machine.name, style: const TextStyle(fontWeight: FontWeight.w600)),
              subtitle: Text(l.machine.lastHost ?? l.machine.hosts.join(', ')),
            ),
            if (l.machine.macs.isNotEmpty)
              ListTile(
                leading: const Icon(Icons.power_settings_new_rounded),
                title: const Text('Разбудить (Wake-on-LAN)'),
                onTap: () async {
                  Navigator.pop(sheet);
                  final ok = await app.wake(l.machine.id);
                  if (context.mounted) {
                    ScaffoldMessenger.of(context)
                        .showSnackBar(SnackBar(content: Text(ok ? 'Сигнал пробуждения отправлен' : 'Не удалось отправить сигнал')));
                  }
                },
              ),
            ListTile(
              leading: const Icon(Icons.refresh_rounded),
              title: const Text('Переподключиться'),
              onTap: () {
                Navigator.pop(sheet);
                l.reconnectNow();
              },
            ),
            ListTile(
              leading: const Icon(Icons.delete_outline_rounded, color: C.danger),
              title: const Text('Удалить с телефона', style: TextStyle(color: C.danger)),
              onTap: () {
                Navigator.pop(sheet);
                app.removeMachine(l.machine.id);
              },
            ),
            const SizedBox(height: 8),
          ],
        ),
      ),
    );
  }
}

class _MachineCard extends StatelessWidget {
  const _MachineCard({required this.link, required this.active, required this.onTap, required this.onLongPress});
  final MachineLink link;
  final bool active;
  final VoidCallback onTap;
  final VoidCallback onLongPress;

  @override
  Widget build(BuildContext context) {
    final m = link.machine;
    final buttons = link.state?.buttons.length;
    return Material(
      color: active ? C.tilePressed : C.tile,
      borderRadius: BorderRadius.circular(18),
      child: InkWell(
        borderRadius: BorderRadius.circular(18),
        onTap: onTap,
        onLongPress: onLongPress,
        child: Container(
          padding: const EdgeInsets.all(16),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(18),
            border: Border.all(color: active ? C.accent : Colors.transparent, width: 1.5),
          ),
          child: Row(
            children: [
              Container(
                width: 48,
                height: 48,
                decoration: BoxDecoration(color: C.bg, borderRadius: BorderRadius.circular(14)),
                child: Icon(m.os == 'macos' ? Icons.laptop_mac_rounded : Icons.desktop_windows_rounded, color: C.text),
              ),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      m.name,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
                    ),
                    const SizedBox(height: 4),
                    Row(
                      children: [
                        StatusDot(link.status, size: 8),
                        const SizedBox(width: 6),
                        Flexible(
                          child: Text(
                            [statusText(link.status), if (buttons != null) '$buttons кн.'].join(' · '),
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: const TextStyle(color: C.muted, fontSize: 13),
                          ),
                        ),
                      ],
                    ),
                  ],
                ),
              ),
              if (active) const Icon(Icons.check_circle_rounded, color: C.accent),
            ],
          ),
        ),
      ),
    );
  }
}
