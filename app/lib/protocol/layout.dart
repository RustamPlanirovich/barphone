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

/// Smallest tile worth keeping a row for in landscape.
const minLandscapeTile = 56.0;

/// Fits square tiles into [width] x [height]. [shortSideTiles] comes from the PC ("columns")
/// and is how many tiles go across the phone's short side.
///
/// Portrait: that many columns, as many rows as fit. Landscape: that many rows, and as many
/// columns as fit (tiles shrink a little to fill the width). With the rows fixed, page dots
/// or a banner never cost a whole row, and half the screen (two computers side by side)
/// simply gets half the columns.
GridLayout computeGrid(double width, double height, int shortSideTiles, {double gap = 12, bool? landscape}) {
  final n = max(1, shortSideTiles);
  if (!(landscape ?? width > height)) {
    var tile = (width - gap * (n + 1)) / n;
    var rows = ((height - gap) / (tile + gap)).floor();
    if (rows < 1) {
      rows = 1;
      tile = min(tile, height - 2 * gap);
    }
    return GridLayout(n, rows, max(tile, 1), gap);
  }
  double rowFit(int r) => (height - gap * (r + 1)) / r;
  var rows = n;
  while (rows > 1 && rowFit(rows) < minLandscapeTile) {
    rows--;
  }
  final fit = rowFit(rows);
  final cols = max(1, ((width - gap) / (fit + gap)).round());
  final tile = min(fit, (width - gap * (cols + 1)) / cols);
  return GridLayout(cols, rows, max(tile, 1), gap);
}
