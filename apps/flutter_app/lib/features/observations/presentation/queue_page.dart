import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../../../core/l10n/strings.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/shell/offline_queue.dart';
import '../../../shared_widgets/shared_widgets.dart';
import '../data/draft_queue.dart';

// Mounted by the shell so automatic retry runs while any app screen is open.
class DraftQueueHost extends ConsumerWidget {
  const DraftQueueHost({super.key, required this.child});
  final Widget child;
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final owner = ref.watch(sessionProvider.select((s) => s.user?.id));
    final data = owner == null ? null : ref.watch(draftQueueProvider(owner));
    final count =
        data?.value
            ?.where(
              (d) =>
                  d.status != DraftStatus.sent && d.status != DraftStatus.draft,
            )
            .length ??
        0;
    Future.microtask(() {
      if (!context.mounted) return;
      final hook = ref.read(offlineQueueProvider.notifier);
      final current = ref.read(offlineQueueProvider);
      if (current.supported != (data?.hasValue ?? false)) {
        hook.setSupported(data?.hasValue ?? false);
      }
      if (current.pending != count) hook.setPending(count);
    });
    return child;
  }
}

class FieldQueuePage extends ConsumerWidget {
  const FieldQueuePage({super.key});
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final owner = ref.watch(sessionProvider).user?.id;
    if (owner == null) return const SizedBox.shrink();
    final s = context.s;
    return Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 850),
        child: Column(
          children: [
            ListTile(
              title: Text(
                s.pick('Saha kayıtlarım', 'My field records'),
                style: Theme.of(context).textTheme.headlineSmall,
              ),
              subtitle: Text(
                s.pick(
                  'Kayıtlar bu cihaza ve hesabınıza bağlı saklanır.',
                  'Records are stored on this device for your account.',
                ),
              ),
              trailing: IconButton(
                tooltip: s.retry,
                onPressed: () => unawaited(
                  ref
                      .read(draftQueueProvider(owner).notifier)
                      .sync(force: true),
                ),
                icon: const Icon(Icons.sync),
              ),
            ),
            Expanded(
              child: ref
                  .watch(draftQueueProvider(owner))
                  .when(
                    loading: () =>
                        const Center(child: CircularProgressIndicator()),
                    error: (e, _) => ErrorState(
                      error: e,
                      onRetry: () => ref.invalidate(draftQueueProvider(owner)),
                    ),
                    data: (items) => items.isEmpty
                        ? EmptyState(
                            icon: Icons.inventory_2_outlined,
                            title: s.pick(
                              'Henüz saha kaydı yok',
                              'No field records yet',
                            ),
                            action: FilledButton(
                              onPressed: () => context.go('/observations/new'),
                              child: Text(s.navAddObservation),
                            ),
                          )
                        : ListView.builder(
                            padding: const EdgeInsets.all(16),
                            itemCount: items.length,
                            itemBuilder: (context, i) {
                              final d = items[items.length - i - 1];
                              final label = s.pick(
                                switch (d.status) {
                                  DraftStatus.draft => 'Taslak',
                                  DraftStatus.queued => 'Bekliyor',
                                  DraftStatus.sending => 'Gönderiliyor',
                                  DraftStatus.sent => 'Gönderildi',
                                  DraftStatus.failed => 'Hata',
                                },
                                switch (d.status) {
                                  DraftStatus.draft => 'Draft',
                                  DraftStatus.queued => 'Waiting',
                                  DraftStatus.sending => 'Sending',
                                  DraftStatus.sent => 'Sent',
                                  DraftStatus.failed => 'Error',
                                },
                              );
                              return Card(
                                child: Padding(
                                  padding: const EdgeInsets.all(16),
                                  child: Column(
                                    crossAxisAlignment:
                                        CrossAxisAlignment.start,
                                    children: [
                                      Text(
                                        d.body['title'] as String? ??
                                            s.pick(
                                              'Başlıksız taslak',
                                              'Untitled draft',
                                            ),
                                        style: Theme.of(
                                          context,
                                        ).textTheme.titleMedium,
                                      ),
                                      const SizedBox(height: 8),
                                      Text(
                                        '$label · ${d.uploaded}/${d.photos.length < d.uploaded ? d.uploaded : d.photos.length} ${s.pick('fotoğraf', 'photos')}',
                                      ),
                                      if (d.error.isNotEmpty)
                                        Text(
                                          s.pick(
                                            'Gönderilemedi. Bağlantıyı ve kayıt bilgilerini kontrol edip tekrar deneyin.',
                                            'Could not send. Check the connection and record details, then retry.',
                                          ),
                                        ),
                                      Wrap(
                                        spacing: 8,
                                        children: [
                                          if (d.status == DraftStatus.draft ||
                                              (d.status == DraftStatus.failed &&
                                                  d.serverId == null))
                                            TextButton(
                                              onPressed: () => context.go(
                                                '/observations/new?draft=${d.id}',
                                              ),
                                              child: Text(
                                                s.pick('Düzenle', 'Edit'),
                                              ),
                                            ),
                                          if (d.status == DraftStatus.failed ||
                                              d.status == DraftStatus.queued)
                                            TextButton(
                                              onPressed: () => ref
                                                  .read(
                                                    draftQueueProvider(
                                                      owner,
                                                    ).notifier,
                                                  )
                                                  .retry(d.id),
                                              child: Text(s.retry),
                                            ),
                                          if (d.serverId != null)
                                            TextButton(
                                              onPressed: () => context.go(
                                                '/observations/${d.serverId}',
                                              ),
                                              child: Text(
                                                s.pick(
                                                  'Kaydı aç',
                                                  'Open record',
                                                ),
                                              ),
                                            ),
                                          if (d.status != DraftStatus.sending)
                                            TextButton(
                                              onPressed: () async {
                                                final ok = await showDialog<bool>(
                                                  context: context,
                                                  builder: (ctx) => AlertDialog(
                                                    title: Text(
                                                      s.pick(
                                                        'Cihazdaki kaydı sil?',
                                                        'Delete the local record?',
                                                      ),
                                                    ),
                                                    content: Text(
                                                      s.pick(
                                                        'Gönderilmiş kayıtlar sunucudan silinmez. Gönderilmemiş fotoğraflar bu cihazdan kaldırılır.',
                                                        'Sent records remain on the server. Unsent photos are removed from this device.',
                                                      ),
                                                    ),
                                                    actions: [
                                                      TextButton(
                                                        onPressed: () =>
                                                            Navigator.pop(
                                                              ctx,
                                                              false,
                                                            ),
                                                        child: Text(s.cancel),
                                                      ),
                                                      FilledButton(
                                                        onPressed: () =>
                                                            Navigator.pop(
                                                              ctx,
                                                              true,
                                                            ),
                                                        child: Text(s.delete),
                                                      ),
                                                    ],
                                                  ),
                                                );
                                                if (ok == true) {
                                                  await ref
                                                      .read(
                                                        draftQueueProvider(
                                                          owner,
                                                        ).notifier,
                                                      )
                                                      .remove(d.id);
                                                }
                                              },
                                              child: Text(s.delete),
                                            ),
                                        ],
                                      ),
                                    ],
                                  ),
                                ),
                              );
                            },
                          ),
                  ),
            ),
          ],
        ),
      ),
    );
  }
}
