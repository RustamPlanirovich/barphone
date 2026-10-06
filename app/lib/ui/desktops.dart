import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../protocol/link.dart';
import '../protocol/models.dart';
import 'glyphs.dart';
import 'theme.dart';
import 'tile.dart' show showPressError;

/// The face of the virtual desktops tile: which desktop is in front ("2 / 5" and dots, or
/// its name), sliding sideways when it changes; the glyph while the PC does not tell.
class DesktopsFace extends StatefulWidget {
  const DesktopsFace({super.key, required this.button, required this.link, required this.size});
  final DeckButton button;
  final MachineLink link;
  final double size;

  @override
  State<DesktopsFace> createState() => _DesktopsFaceState();
}

class _DesktopsFaceState extends State<DesktopsFace> {
  int _last = 0; // the desktop shown before: which way to slide

  @override
  Widget build(BuildContext context) {
    final s = widget.size;
    return ValueListenableBuilder<DesktopInfo?>(
      valueListenable: widget.link.desktops,
      builder: (context, d, _) {
        if (d == null) return GlyphIcon('desktops', size: s * .5);
        final forward = d.current >= _last;
        _last = d.current;
        final named = d.current < d.names.length && d.names[d.current].isNotEmpty;
        return SizedBox(
          width: s * .8,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              SizedBox(
                height: s * .3,
                child: ClipRect(
                  child: AnimatedSwitcher(
                    duration: const Duration(milliseconds: 220),
                    transitionBuilder: (child, anim) {
                      final incoming = child.key == ValueKey(d.current);
                      final from = Offset((incoming == forward) ? 1 : -1, 0);
                      return SlideTransition(
                        position: Tween(begin: from, end: Offset.zero).animate(anim),
                        child: child,
                      );
                    },
                    child: FittedBox(
                      key: ValueKey(d.current),
                      child: Text.rich(
                        TextSpan(
                          text: '${d.current + 1}',
                          style: const TextStyle(fontWeight: FontWeight.w700),
                          children: [
                            TextSpan(
                              text: ' / ${d.count}',
                              style: const TextStyle(color: C.muted, fontWeight: FontWeight.w400),
                            ),
                          ],
                        ),
                      ),
                    ),
                  ),
                ),
              ),
              SizedBox(height: s * .05),
              if (named)
                Text(
                  d.names[d.current],
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(fontSize: (s * .09).clamp(9.0, 13.0), color: C.accent),
                )
              else
                _Dots(count: d.count, current: d.current, size: s),
            ],
          ),
        );
      },
    );
  }
}

class _Dots extends StatelessWidget {
  const _Dots({required this.count, required this.current, required this.size});
  final int count, current;
  final double size;

  @override
  Widget build(BuildContext context) {
    if (count > 9) return const SizedBox.shrink(); // the number says it all
    final dot = (size * .045).clamp(4.0, 7.0);
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        for (var i = 0; i < count; i++)
          AnimatedContainer(
            duration: const Duration(milliseconds: 200),
            margin: EdgeInsets.symmetric(horizontal: dot * .35),
            width: i == current ? dot * 2.4 : dot,
            height: dot,
            decoration: BoxDecoration(color: i == current ? C.accent : C.border, borderRadius: BorderRadius.circular(dot)),
          ),
      ],
    );
  }
}

/// Long press on the desktops tile: all desktops as tiles; a tap goes straight there.
Future<void> showDesktopChooser(BuildContext context, MachineLink link, DeckButton b) async {
  HapticFeedback.mediumImpact();
  final d = link.desktops.value;
  if (d == null) {
    // The PC does not say (a Mac): only the overview is possible.
    final r = await link.desktop(b.id, overview: true);
    if (!r.ok && context.mounted) showPressError(context, b, r.error);
    return;
  }
  final to = await showModalBottomSheet<int>(
    context: context,
    showDragHandle: true,
    isScrollControlled: true,
    constraints: BoxConstraints(maxHeight: MediaQuery.sizeOf(context).height * .85, maxWidth: 900),
    builder: (sheet) => SafeArea(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(16, 0, 16, 16),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text('Рабочие столы · ${link.machine.name}', style: const TextStyle(fontSize: 17, fontWeight: FontWeight.w600)),
            const SizedBox(height: 12),
            Flexible(
              child: GridView(
                shrinkWrap: true,
                gridDelegate: const SliverGridDelegateWithMaxCrossAxisExtent(
                  maxCrossAxisExtent: 160,
                  mainAxisSpacing: 10,
                  crossAxisSpacing: 10,
                  childAspectRatio: 1.4,
                ),
                children: [
                  for (var i = 0; i < d.count; i++)
                    Material(
                      color: i == d.current ? C.tilePressed : C.tile,
                      shape: RoundedRectangleBorder(
                        borderRadius: BorderRadius.circular(16),
                        side: BorderSide(color: i == d.current ? C.accent : Colors.transparent, width: 1.5),
                      ),
                      child: InkWell(
                        customBorder: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
                        onTap: () => Navigator.pop(sheet, i),
                        child: Padding(
                          padding: const EdgeInsets.all(10),
                          child: Column(
                            mainAxisAlignment: MainAxisAlignment.center,
                            children: [
                              Text('${i + 1}', style: const TextStyle(fontSize: 26, fontWeight: FontWeight.w700)),
                              Text(
                                d.name(i),
                                maxLines: 1,
                                overflow: TextOverflow.ellipsis,
                                style: TextStyle(fontSize: 12, color: i == d.current ? C.accent : C.muted),
                              ),
                            ],
                          ),
                        ),
                      ),
                    ),
                ],
              ),
            ),
            const SizedBox(height: 12),
            OutlinedButton.icon(
              onPressed: () => Navigator.pop(sheet, -1),
              icon: const Icon(Icons.grid_view_rounded),
              label: const Text('Обзор всех окон и столов'),
            ),
          ],
        ),
      ),
    ),
  );
  if (to == null || to == d.current) return;
  HapticFeedback.selectionClick();
  final r = to < 0 ? await link.desktop(b.id, overview: true) : await link.desktop(b.id, to: to);
  if (!r.ok && context.mounted) showPressError(context, b, r.error);
}
