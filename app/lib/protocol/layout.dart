// Deck grid geometry. Pure Dart.
import 'dart:math';

class GridLayout {
  final int columns;
  final int rows;
  final double tile;
  final double gap;

  const GridLayout(this.columns, this.rows, this.tile, this.gap);

  int get perPage => columns * rows;

  int pages(int buttons) => max(1, (buttons / perPage).ceil());
}

/// Fits square tiles into [width] x [height]. [portraitColumns] comes from the PC; in
/// landscape the phone adds columns so tiles keep roughly the same size.
GridLayout computeGrid(double width, double height, int portraitColumns, {double gap = 12}) {
  var cols = max(1, portraitColumns);
  if (width > height && height > 0) {
    cols = max(cols, (cols * width / height).round());
  }
  var tile = (width - gap * (cols + 1)) / cols;
  var rows = ((height - gap) / (tile + gap)).floor();
  if (rows < 1) {
    rows = 1;
    tile = min(tile, height - 2 * gap);
  }
  return GridLayout(cols, rows, max(tile, 1), gap);
}
