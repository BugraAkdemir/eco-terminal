import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../../core/api/providers.dart';
import '../../../core/l10n/strings.dart';
import '../../../core/models/models.dart';
import '../../../shared_widgets/shared_widgets.dart';
import '../../maps/data/map_providers.dart';
import '../../maps/presentation/observation_detail.dart';

final reviewQueueProvider =
    FutureProvider.family<Paged<ObservationDetail>, String>(
      (ref, category) => ref
          .watch(reviewApiProvider)
          .queue(category: category.isEmpty ? null : category, pageSize: 200),
    );

class ExpertPage extends ConsumerStatefulWidget {
  const ExpertPage({super.key});
  @override
  ConsumerState<ExpertPage> createState() => _ExpertState();
}

class _ExpertState extends ConsumerState<ExpertPage> {
  String category = '';
  int selected = 0;
  bool busy = false;
  final note = TextEditingController(), species = TextEditingController();
  bool sensitive = false;
  String? editingId;
  @override
  void dispose() {
    note.dispose();
    species.dispose();
    super.dispose();
  }

  Future<void> decide(
    ObservationDetail obs,
    ReviewDecisionType decision,
  ) async {
    if ((decision == ReviewDecisionType.reject ||
            decision == ReviewDecisionType.requestInfo) &&
        note.text.trim().isEmpty) {
      showErrorSnack(
        context,
        context.s.pick(
          'Kararın nedenini yazın.',
          'Give a reason for this decision.',
        ),
      );
      return;
    }
    if (decision == ReviewDecisionType.verify ||
        decision == ReviewDecisionType.reject) {
      final ok = await showDialog<bool>(
        context: context,
        builder: (ctx) => AlertDialog(
          title: Text(
            context.s.pick(
              'Uzman kararını kaydet?',
              'Save the expert decision?',
            ),
          ),
          content: Text(obs.displayTitle),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx, false),
              child: Text(context.s.cancel),
            ),
            FilledButton(
              onPressed: () => Navigator.pop(ctx, true),
              child: Text(context.s.confirm),
            ),
          ],
        ),
      );
      if (ok != true) return;
    }
    setState(() => busy = true);
    try {
      await ref
          .read(reviewApiProvider)
          .decide(
            obs.id,
            ReviewDecision(
              decision: decision,
              note: note.text.trim(),
              correctedSpecies: species.text.trim().isEmpty
                  ? null
                  : species.text.trim(),
              markSensitive: sensitive,
            ),
          );
      ref.invalidate(reviewQueueProvider(category));
      ref.invalidate(observationDetailProvider(obs.id));
      ref.invalidate(viewportObservationsProvider);
      note.clear();
      editingId = null;
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = context.s;
    final queue = ref.watch(reviewQueueProvider(category));
    return Column(
      children: [
        ListTile(
          title: Text(
            s.pick('Uzman inceleme masası', 'Expert review desk'),
            style: Theme.of(context).textTheme.headlineSmall,
          ),
          subtitle: Text(
            s.pick(
              'J/K: sonraki/önceki · V: onay · R: ret',
              'J/K: next/previous · V: verify · R: reject',
            ),
          ),
          trailing: IconButton(
            tooltip: s.retry,
            onPressed: () => ref.invalidate(reviewQueueProvider(category)),
            icon: const Icon(Icons.refresh),
          ),
        ),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16),
          child: DropdownButtonFormField<String>(
            initialValue: category,
            decoration: InputDecoration(
              labelText: s.pick('Kategori', 'Category'),
            ),
            items: [
              DropdownMenuItem(value: '', child: Text(s.all)),
              ...ObservationCategory.values.map(
                (c) => DropdownMenuItem(
                  value: c.wire,
                  child: Text(s.categoryLabel(c)),
                ),
              ),
            ],
            onChanged: (v) => setState(() {
              category = v ?? '';
              selected = 0;
              editingId = null;
            }),
          ),
        ),
        Expanded(
          child: queue.when(
            loading: () => const Center(child: CircularProgressIndicator()),
            error: (e, _) => ErrorState(
              error: e,
              onRetry: () => ref.invalidate(reviewQueueProvider(category)),
            ),
            data: (page) {
              if (page.items.isEmpty) {
                return EmptyState(
                  icon: Icons.task_alt,
                  title: s.pick(
                    'İnceleme kuyruğu boş',
                    'Review queue is empty',
                  ),
                );
              }
              selected = selected.clamp(0, page.items.length - 1);
              final obs = page.items[selected];
              if (editingId != obs.id) {
                editingId = obs.id;
                species.text = obs.speciesVerified;
                sensitive = obs.isSensitive;
                note.clear();
              }
              final details = Column(
                children: [
                  Expanded(child: ObservationDetailBody(detail: obs)),
                  Padding(
                    padding: const EdgeInsets.all(16),
                    child: Column(
                      children: [
                        TextField(
                          controller: species,
                          decoration: InputDecoration(
                            labelText: s.pick(
                              'Uzmanın tür düzeltmesi',
                              'Expert species correction',
                            ),
                          ),
                        ),
                        TextField(
                          controller: note,
                          maxLines: 2,
                          decoration: InputDecoration(
                            labelText: s.pick('Karar notu', 'Decision note'),
                          ),
                        ),
                        CheckboxListTile(
                          contentPadding: EdgeInsets.zero,
                          value: sensitive,
                          onChanged: busy
                              ? null
                              : (v) => setState(() => sensitive = v ?? false),
                          title: Text(
                            s.pick(
                              'Hassas tür: konumu genelleştir',
                              'Sensitive species: generalize location',
                            ),
                          ),
                        ),
                        Wrap(
                          spacing: 8,
                          runSpacing: 8,
                          children: [
                            for (final d in [
                              ReviewDecisionType.startReview,
                              ReviewDecisionType.verify,
                              ReviewDecisionType.requestInfo,
                              ReviewDecisionType.reject,
                            ])
                              OutlinedButton(
                                onPressed: busy ? null : () => decide(obs, d),
                                child: Text(
                                  s.pick(
                                    switch (d) {
                                      ReviewDecisionType.startReview =>
                                        'İncelemeye başla',
                                      ReviewDecisionType.verify => 'Onayla',
                                      ReviewDecisionType.requestInfo =>
                                        'Ek bilgi iste',
                                      _ => 'Reddet',
                                    },
                                    switch (d) {
                                      ReviewDecisionType.startReview =>
                                        'Start review',
                                      ReviewDecisionType.verify => 'Verify',
                                      ReviewDecisionType.requestInfo =>
                                        'Request information',
                                      _ => 'Reject',
                                    },
                                  ),
                                ),
                              ),
                          ],
                        ),
                      ],
                    ),
                  ),
                ],
              );
              final list = ListView.builder(
                itemCount: page.items.length,
                itemBuilder: (context, i) => ListTile(
                  selected: i == selected,
                  title: Text(page.items[i].displayTitle),
                  subtitle: Text(s.statusLabel(page.items[i].status)),
                  onTap: () => setState(() => selected = i),
                ),
              );
              final navigableList = CallbackShortcuts(
                bindings: {
                  const SingleActivator(LogicalKeyboardKey.keyJ): () =>
                      setState(
                        () => selected = (selected + 1).clamp(
                          0,
                          page.items.length - 1,
                        ),
                      ),
                  const SingleActivator(LogicalKeyboardKey.keyK): () =>
                      setState(
                        () => selected = (selected - 1).clamp(
                          0,
                          page.items.length - 1,
                        ),
                      ),
                  const SingleActivator(LogicalKeyboardKey.keyV): () {
                    if (!busy) decide(obs, ReviewDecisionType.verify);
                  },
                  const SingleActivator(LogicalKeyboardKey.keyR): () {
                    if (!busy) decide(obs, ReviewDecisionType.reject);
                  },
                },
                child: Focus(autofocus: true, child: list),
              );
              return LayoutBuilder(
                builder: (context, c) => c.maxWidth >= 900
                    ? Row(
                        children: [
                          SizedBox(width: 280, child: navigableList),
                          const VerticalDivider(),
                          Expanded(child: details),
                        ],
                      )
                    : Column(
                        children: [
                          SizedBox(height: 140, child: navigableList),
                          Expanded(child: details),
                        ],
                      ),
              );
            },
          ),
        ),
      ],
    );
  }
}
