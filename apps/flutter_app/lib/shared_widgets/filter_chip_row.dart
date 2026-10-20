import 'package:flutter/material.dart';

import '../core/theme/tokens.dart';

class FilterOption<T> {
  const FilterOption({
    required this.value,
    required this.label,
    this.icon,
    this.color,
  });
  final T value;
  final String label;
  final IconData? icon;
  final Color? color;
}

/// Horizontally scrolling row of choice chips.
/// - [multiSelect] false: single selection; tapping the selected chip clears it
///   (when [allowClear]).
/// - [multiSelect] true: [selected] is a set; empty set means "all".
class FilterChipRow<T> extends StatelessWidget {
  const FilterChipRow({
    super.key,
    required this.options,
    required this.selected,
    required this.onChanged,
    this.multiSelect = false,
    this.allowClear = true,
  });

  final List<FilterOption<T>> options;
  final Set<T> selected;
  final ValueChanged<Set<T>> onChanged;
  final bool multiSelect;
  final bool allowClear;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    return SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      child: Row(
        children: [
          for (final o in options)
            Padding(
              padding: const EdgeInsets.only(right: Dimens.sm),
              child: FilterChip(
                selected: selected.contains(o.value),
                avatar: o.icon == null
                    ? null
                    : Icon(o.icon, size: 16, color: o.color ?? eco.textMuted),
                label: Text(o.label),
                selectedColor: (o.color ?? eco.teal).withValues(alpha: 0.2),
                side: BorderSide(
                  color: selected.contains(o.value)
                      ? (o.color ?? eco.teal)
                      : eco.border,
                ),
                onSelected: (on) {
                  final next = Set<T>.of(selected);
                  if (multiSelect) {
                    on ? next.add(o.value) : next.remove(o.value);
                  } else {
                    next.clear();
                    if (on || !allowClear) next.add(o.value);
                  }
                  onChanged(next);
                },
              ),
            ),
        ],
      ),
    );
  }
}
