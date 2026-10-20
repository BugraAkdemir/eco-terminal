import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:latlong2/latlong.dart';

import '../../features/maps/domain/map_state.dart';
import '../../features/maps/presentation/map_stack.dart';
import '../api/providers.dart';
import '../api/services/observations_api.dart';
import '../l10n/strings.dart';
import '../models/models.dart';
import '../theme/tokens.dart';

/// Debounced autocomplete over an async source. The latest query wins;
/// failures yield an empty list (the field never throws into the UI).
class AsyncSearchField<T extends Object> extends StatefulWidget {
  const AsyncSearchField({
    super.key,
    required this.hint,
    required this.icon,
    required this.search,
    required this.titleOf,
    required this.onSelected,
    this.subtitleOf,
    this.autofocus = false,
    this.minChars = 2,
    this.debounce = const Duration(milliseconds: 350),
  });

  final String hint;
  final IconData icon;
  final Future<List<T>> Function(String query) search;
  final String Function(T) titleOf;
  final String? Function(T)? subtitleOf;
  final ValueChanged<T> onSelected;
  final bool autofocus;
  final int minChars;
  final Duration debounce;

  @override
  State<AsyncSearchField<T>> createState() => _AsyncSearchFieldState<T>();
}

class _AsyncSearchFieldState<T extends Object>
    extends State<AsyncSearchField<T>> {
  int _seq = 0;
  final _focus = FocusNode();
  final _controller = TextEditingController();

  @override
  void dispose() {
    _focus.dispose();
    _controller.dispose();
    super.dispose();
  }

  Future<Iterable<T>> _options(TextEditingValue v) async {
    final q = v.text.trim();
    final my = ++_seq;
    if (q.length < widget.minChars) return const [];
    await Future<void>.delayed(widget.debounce);
    if (my != _seq) return const [];
    try {
      return await widget.search(q);
    } catch (_) {
      return const [];
    }
  }

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    return RawAutocomplete<T>(
      focusNode: _focus,
      textEditingController: _controller,
      optionsBuilder: _options,
      displayStringForOption: widget.titleOf,
      onSelected: (v) {
        widget.onSelected(v);
        _controller.clear();
        _focus.unfocus();
      },
      fieldViewBuilder: (context, controller, focusNode, onSubmit) {
        return SizedBox(
          height: 40,
          child: TextField(
            controller: controller,
            focusNode: focusNode,
            autofocus: widget.autofocus,
            textInputAction: TextInputAction.search,
            decoration: InputDecoration(
              hintText: widget.hint,
              prefixIcon: Icon(widget.icon, size: 18),
              contentPadding: const EdgeInsets.symmetric(vertical: 0),
              suffixIcon: ValueListenableBuilder<TextEditingValue>(
                valueListenable: controller,
                builder: (_, v, _) => v.text.isEmpty
                    ? const SizedBox.shrink()
                    : IconButton(
                        icon: const Icon(Icons.close, size: 16),
                        onPressed: controller.clear,
                      ),
              ),
            ),
          ),
        );
      },
      optionsViewBuilder: (context, onSelected, options) {
        return Align(
          alignment: Alignment.topLeft,
          child: Material(
            elevation: 6,
            color: eco.surface,
            borderRadius: BorderRadius.circular(Dimens.radius),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 420, maxHeight: 320),
              child: ListView.builder(
                padding: EdgeInsets.zero,
                shrinkWrap: true,
                itemCount: options.length,
                itemBuilder: (context, i) {
                  final o = options.elementAt(i);
                  final sub = widget.subtitleOf?.call(o);
                  return ListTile(
                    dense: true,
                    title: Text(
                      widget.titleOf(o),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                    subtitle: sub == null || sub.isEmpty
                        ? null
                        : Text(sub, maxLines: 1, overflow: TextOverflow.ellipsis),
                    onTap: () => onSelected(o),
                  );
                },
              ),
            ),
          ),
        );
      },
    );
  }
}

/// Global search: observations by text (title / description / species).
class GlobalSearchField extends ConsumerWidget {
  const GlobalSearchField({super.key, this.autofocus = false, this.onDone});
  final bool autofocus;
  final VoidCallback? onDone;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = context.s;
    return AsyncSearchField<Observation>(
      hint: s.globalSearchHint,
      icon: Icons.search,
      autofocus: autofocus,
      search: (q) async {
        final res = await ref
            .read(observationsApiProvider)
            .list(ObservationQuery(q: q, pageSize: 8));
        return res.items;
      },
      titleOf: (o) => o.displayTitle,
      subtitleOf: (o) => [
        s.categoryLabel(o.category),
        if (o.primaryRegionName != null) o.primaryRegionName!,
      ].join(' · '),
      onSelected: (o) {
        focusObservation(ref, o.id, o.lat, o.lon);
        onDone?.call();
      },
    );
  }
}

/// Region search: selects the region (shared context) and fits the map.
class RegionSearchField extends ConsumerWidget {
  const RegionSearchField({super.key, this.autofocus = false, this.onDone});
  final bool autofocus;
  final VoidCallback? onDone;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = context.s;
    return AsyncSearchField<Region>(
      hint: s.regionSearchHint,
      icon: Icons.travel_explore,
      autofocus: autofocus,
      search: (q) => ref.read(regionsApiProvider).list(q: q),
      titleOf: (r) => r.name,
      subtitleOf: (r) => r.kind.name,
      onSelected: (r) {
        ref.read(selectedRegionProvider.notifier).select(r);
        final focus = ref.read(mapFocusProvider.notifier);
        if (r.hasBbox) {
          focus.fitBounds(r.bbox);
        } else if (r.centroidLat != null && r.centroidLon != null) {
          focus.flyTo(LatLng(r.centroidLat!, r.centroidLon!), zoom: 11);
        }
        onDone?.call();
      },
    );
  }
}

/// Opens a search field in a dialog (header icon on narrow screens).
Future<void> showSearchDialog(BuildContext context, {required bool region}) {
  return showDialog<void>(
    context: context,
    builder: (ctx) => Dialog(
      alignment: Alignment.topCenter,
      insetPadding: const EdgeInsets.fromLTRB(16, 72, 16, 16),
      child: Padding(
        padding: const EdgeInsets.all(Dimens.md),
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 480),
          child: SizedBox(
            height: 300,
            child: Align(
              alignment: Alignment.topCenter,
              child: region
                  ? RegionSearchField(
                      autofocus: true,
                      onDone: () => Navigator.of(ctx).pop(),
                    )
                  : GlobalSearchField(
                      autofocus: true,
                      onDone: () => Navigator.of(ctx).pop(),
                    ),
            ),
          ),
        ),
      ),
    ),
  );
}
