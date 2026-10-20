import 'package:fl_chart/fl_chart.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:url_launcher/url_launcher.dart';
import '../../../core/api/providers.dart';
import '../../../core/l10n/strings.dart';
import '../../../core/models/models.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/theme/tokens.dart';
import '../../../shared_widgets/shared_widgets.dart';
import '../../maps/domain/map_state.dart';

Future<void> openSource(BuildContext context, String? url) async {
  if (url == null) return;
  final uri = Uri.tryParse(url);
  if (uri == null || !['https', 'http'].contains(uri.scheme)) return;
  if (!await launchUrl(uri, mode: LaunchMode.externalApplication) &&
      context.mounted) {
    showErrorSnack(
      context,
      context.s.pick('Kaynak açılamadı.', 'Could not open the source.'),
    );
  }
}

class ContextStrip extends ConsumerWidget {
  const ContextStrip({super.key});
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final region = ref.watch(selectedRegionProvider),
        range = ref.watch(resolvedRangeProvider);
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 12),
      child: Text(
        '${context.s.pick('Bölge', 'Region')}: ${region?.name ?? context.s.pick('Seçilmedi', 'Not selected')}\n${range.from.toIso8601String().substring(0, 10)} → ${range.to.toIso8601String().substring(0, 10)}',
        style: Theme.of(context).textTheme.bodySmall,
      ),
    );
  }
}

class AssistantTab extends ConsumerStatefulWidget {
  const AssistantTab({super.key});
  @override
  ConsumerState<AssistantTab> createState() => _AssistantState();
}

class _AssistantState extends ConsumerState<AssistantTab> {
  final input = TextEditingController();
  final List<(String, ChatAnswer)> messages = [];
  bool busy = false;
  Object? error;
  @override
  void dispose() {
    input.dispose();
    super.dispose();
  }

  Future<void> send() async {
    final question = input.text.trim();
    if (question.isEmpty || busy) return;
    final region = ref.read(selectedRegionProvider),
        range = ref.read(resolvedRangeProvider);
    final token = ref.read(sessionProvider).token;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      final answer = await ref
          .read(aiApiProvider)
          .chat(
            ChatRequest(
              message: question,
              regionId: region?.id,
              from: range.from.toUtc().toIso8601String(),
              to: range.to.toUtc().toIso8601String(),
              history: messages
                  .skip(messages.length > 5 ? messages.length - 5 : 0)
                  .expand(
                    (m) => [
                      ChatHistoryItem(role: 'user', content: m.$1),
                      ChatHistoryItem(
                        role: 'assistant',
                        content: m.$2.answer.length > 2000
                            ? m.$2.answer.substring(0, 2000)
                            : m.$2.answer,
                      ),
                    ],
                  )
                  .toList(),
            ),
          );
      if (!mounted || ref.read(sessionProvider).token != token) return;
      setState(() {
        messages.add((question, answer));
        input.clear();
      });
    } catch (e) {
      if (mounted) setState(() => error = e);
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = context.s;
    final session = ref.watch(sessionProvider);
    ref.listen(sessionProvider.select((s) => s.token), (a, b) {
      if (a != b) setState(() => messages.clear());
    });
    if (!session.isAuthenticated) {
      return EmptyState(
        icon: Icons.login,
        title: s.pick(
          'Soru sormak için giriş yapın',
          'Sign in to ask a question',
        ),
        action: FilledButton(
          onPressed: () => context.go('/login'),
          child: Text(s.pick('Giriş yap', 'Sign in')),
        ),
      );
    }
    return Column(
      children: [
        const Padding(
          padding: EdgeInsets.symmetric(horizontal: 16),
          child: ContextStrip(),
        ),
        Expanded(
          child: ListView(
            padding: const EdgeInsets.all(16),
            children: [
              if (messages.isEmpty) ...[
                Text(
                  s.pick('Sahaya dair bir soru sorun', 'Ask about the field'),
                  style: Theme.of(context).textTheme.headlineSmall,
                ),
                const SizedBox(height: 12),
                Text(
                  s.pick(
                    'Örneğin: “Bu bölgede kaç hayvan gözlemi var?” Sayılar kayıtlı verilerden hesaplanır.',
                    'For example: “How many animal observations are in this region?” Counts come from stored records.',
                  ),
                ),
              ],
              for (final m in messages)
                Card(
                  child: Padding(
                    padding: const EdgeInsets.all(16),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          m.$1,
                          style: Theme.of(context).textTheme.titleMedium,
                        ),
                        const Divider(),
                        Text(
                          m.$2.mode == ChatMode.retrievalOnly
                              ? s.pick(
                                  'Kayıtlı verilerden özet — metin üretimi kapalı',
                                  'Stored-data summary — text generation disabled',
                                )
                              : s.pick(
                                  'Yapay zekâ önerisi — doğrulanmadı',
                                  'AI suggestion — unverified',
                                ),
                          style: TextStyle(color: context.eco.warning),
                        ),
                        if (m.$2.region != null) Text(m.$2.region!.name),
                        const SizedBox(height: 12),
                        SelectableText(m.$2.answer),
                        const SizedBox(height: 12),
                        Wrap(
                          spacing: 8,
                          runSpacing: 8,
                          children: m.$2.sources
                              .map(
                                (source) => SourceCitationChip(
                                  source: source,
                                  onTap: () {
                                    if (source.type == 'observation' &&
                                        source.entityId != null) {
                                      context.go(
                                        '/observations/${source.entityId}',
                                      );
                                    } else if (source.url != null) {
                                      openSource(context, source.url);
                                    } else {
                                      showDialog<void>(
                                        context: context,
                                        builder: (ctx) => AlertDialog(
                                          title: Text(source.title),
                                          content: Text(source.snippet),
                                          actions: [
                                            TextButton(
                                              onPressed: () =>
                                                  Navigator.pop(ctx),
                                              child: Text(s.close),
                                            ),
                                          ],
                                        ),
                                      );
                                    }
                                  },
                                ),
                              )
                              .toList(),
                        ),
                        for (final limitation in m.$2.limitations)
                          Padding(
                            padding: const EdgeInsets.only(top: 8),
                            child: Text(
                              limitation,
                              style: Theme.of(context).textTheme.bodySmall,
                            ),
                          ),
                      ],
                    ),
                  ),
                ),
              if (error != null)
                ErrorState(error: error!, compact: true, onRetry: send),
              if (busy) const LinearProgressIndicator(),
            ],
          ),
        ),
        Padding(
          padding: const EdgeInsets.all(16),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.end,
            children: [
              Expanded(
                child: TextField(
                  key: const ValueKey('assistant-question'),
                  controller: input,
                  maxLength: 2000,
                  minLines: 1,
                  maxLines: 3,
                  decoration: InputDecoration(
                    labelText: s.pick('Sorunuz', 'Your question'),
                  ),
                  onSubmitted: (_) => send(),
                ),
              ),
              const SizedBox(width: 8),
              IconButton.filled(
                key: const ValueKey('assistant-send'),
                tooltip: s.send,
                onPressed: busy ? null : send,
                icon: const Icon(Icons.send),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

final feedArticlesProvider = FutureProvider<Paged<FeedArticle>>((ref) {
  final region = ref.watch(selectedRegionProvider),
      range = ref.watch(resolvedRangeProvider);
  return ref
      .watch(feedsApiProvider)
      .articles(
        regionId: region?.id,
        from: range.from,
        to: range.to,
        pageSize: 50,
      );
});
final feedSourcesProvider = FutureProvider<List<FeedSource>>(
  (ref) => ref.watch(feedsApiProvider).sources(),
);

class FeedsTab extends ConsumerWidget {
  const FeedsTab({super.key});
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = context.s;
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Text(
          s.pick('Kaynağından duyurular', 'Updates from their sources'),
          style: Theme.of(context).textTheme.headlineSmall,
        ),
        const ContextStrip(),
        Text(
          s.pick(
            'Harita bölgesi ve tarih aralığına göre süzülür. Konumu belirlenemeyen duyurular bir bölgeye atanmaz.',
            'Filtered by map region and dates. Updates without a known location are not assigned to a region.',
          ),
        ),
        const SizedBox(height: 12),
        ref
            .watch(feedArticlesProvider)
            .when(
              loading: () => const LinearProgressIndicator(),
              error: (e, _) => ErrorState(
                error: e,
                compact: true,
                onRetry: () => ref.invalidate(feedArticlesProvider),
              ),
              data: (page) => Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  if (page.items.isEmpty)
                    EmptyState(
                      icon: Icons.newspaper,
                      title: s.pick(
                        'Bu aralıkta duyuru yok',
                        'No updates for this period',
                      ),
                    ),
                  for (final article in page.items)
                    Card(
                      child: Padding(
                        padding: const EdgeInsets.all(16),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              article.sourceName,
                              style: TextStyle(color: context.eco.teal),
                            ),
                            Text(
                              article.title,
                              style: Theme.of(context).textTheme.titleMedium,
                            ),
                            const SizedBox(height: 8),
                            Text(article.summary),
                            if (article.locationNote.isNotEmpty)
                              Text(article.locationNote),
                            TextButton.icon(
                              onPressed: () => openSource(context, article.url),
                              icon: const Icon(Icons.open_in_new),
                              label: Text(
                                s.pick(
                                  'Asıl kaynağı aç',
                                  'Open original source',
                                ),
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                ],
              ),
            ),
        const Divider(),
        Text(
          s.pick('Kaynak durumu', 'Source health'),
          style: Theme.of(context).textTheme.titleMedium,
        ),
        ref
            .watch(feedSourcesProvider)
            .when(
              loading: () => const LinearProgressIndicator(),
              error: (e, _) => ErrorState(
                error: e,
                compact: true,
                onRetry: () => ref.invalidate(feedSourcesProvider),
              ),
              data: (sources) => Column(
                children: [
                  for (final source in sources)
                    ListTile(
                      contentPadding: EdgeInsets.zero,
                      title: Text(source.name),
                      subtitle: Text(
                        '${source.organization}\n${source.lastStatus.isEmpty ? s.pick('Henüz çekilmedi', 'Not fetched yet') : source.lastStatus}',
                      ),
                      trailing: const Icon(Icons.rss_feed),
                      onTap: () => openSource(context, source.feedUrl),
                    ),
                ],
              ),
            ),
        if (ref.watch(sessionProvider).role == UserRole.admin)
          OutlinedButton.icon(
            onPressed: () async {
              try {
                await ref.read(feedsApiProvider).sync();
                if (context.mounted) {
                  showSuccessSnack(
                    context,
                    s.pick('Güncelleme kuyruğa alındı.', 'Update queued.'),
                  );
                }
              } catch (e) {
                if (context.mounted) showErrorSnack(context, e);
              }
            },
            icon: const Icon(Icons.sync),
            label: Text(
              s.pick('Kaynakları şimdi güncelle', 'Refresh sources now'),
            ),
          ),
      ],
    );
  }
}

final savedSimulationsProvider = FutureProvider<List<SimulationResult>>((ref) {
  ref.watch(sessionProvider.select((s) => s.token));
  return ref.watch(simulationApiProvider).list();
});

class SimulationsTab extends ConsumerStatefulWidget {
  const SimulationsTab({super.key});
  @override
  ConsumerState<SimulationsTab> createState() => _SimulationsState();
}

class _SimulationsState extends ConsumerState<SimulationsTab> {
  final from = TextEditingController(text: '1991-01'),
      to = TextEditingController(text: '2020-12'),
      name = TextEditingController();
  double precip = -20, temp = 2;
  int scale = 3;
  bool busy = false;
  Object? error;
  SimulationResult? result;
  String? other;
  Map<String, dynamic>? comparison;
  @override
  void dispose() {
    from.dispose();
    to.dispose();
    name.dispose();
    super.dispose();
  }

  Future<void> run() async {
    final region = ref.read(selectedRegionProvider);
    if (region == null) return;
    setState(() {
      busy = true;
      error = null;
      comparison = null;
    });
    try {
      final r = await ref
          .read(simulationApiProvider)
          .run(
            SimulationRunRequest(
              name: name.text.trim(),
              regionId: region.id,
              referenceFrom: from.text.trim(),
              referenceTo: to.text.trim(),
              precipChangePct: precip,
              tempChangeC: temp,
              scale: scale,
              explain: true,
            ),
          );
      if (mounted) setState(() => result = r);
      ref.invalidate(savedSimulationsProvider);
    } catch (e) {
      if (mounted) setState(() => error = e);
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> compare() async {
    if (result == null || other == null) return;
    setState(() => busy = true);
    try {
      final env = await ref
          .read(apiClientProvider)
          .get('/simulations/compare', query: {'a': result!.id, 'b': other});
      if (mounted) setState(() => comparison = env.object);
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = context.s, region = ref.watch(selectedRegionProvider);
    final canRun = ref.watch(sessionProvider).role?.canSimulate ?? false;
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Text(
          s.pick('Yağış değişse ne olur?', 'What if rainfall changes?'),
          style: Theme.of(context).textTheme.headlineSmall,
        ),
        const ContextStrip(),
        Text(
          s.pick(
            'Bu bir senaryo denemesidir; tahmin veya iklim projeksiyonu değildir. Gerçek ERA5 referans verisiyle aynı dönemi farklı yağış ve sıcaklık altında hesaplar.',
            'This is a scenario experiment, not a forecast or climate projection. It recalculates a real ERA5 reference period under changed rainfall and temperature.',
          ),
        ),
        if (!canRun)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 16),
            child: Text(
              s.pick(
                'Senaryo çalıştırmak için araştırmacı veya yönetici hesabı gerekli.',
                'A researcher or administrator account is required to run scenarios.',
              ),
            ),
          ),
        if (canRun) ...[
          const SizedBox(height: 16),
          TextField(
            controller: name,
            decoration: InputDecoration(
              labelText: s.pick(
                'Senaryo adı (isteğe bağlı)',
                'Scenario name (optional)',
              ),
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: from,
            decoration: InputDecoration(
              labelText: s.pick(
                'Referans başlangıcı (YYYY-MM)',
                'Reference start (YYYY-MM)',
              ),
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: to,
            decoration: InputDecoration(
              labelText: s.pick(
                'Referans bitişi (YYYY-MM)',
                'Reference end (YYYY-MM)',
              ),
            ),
          ),
          Text(
            '${s.pick('Yağış değişimi', 'Rainfall change')}: ${precip.toStringAsFixed(0)}%',
          ),
          Slider(
            value: precip,
            min: -80,
            max: 100,
            divisions: 36,
            label: '${precip.round()}%',
            onChanged: busy ? null : (v) => setState(() => precip = v),
          ),
          Text(
            '${s.pick('Sıcaklık değişimi', 'Temperature change')}: ${temp.toStringAsFixed(1)} °C',
          ),
          Slider(
            value: temp,
            min: -5,
            max: 10,
            divisions: 30,
            label: '${temp.toStringAsFixed(1)} °C',
            onChanged: busy ? null : (v) => setState(() => temp = v),
          ),
          DropdownButtonFormField<int>(
            initialValue: scale,
            decoration: InputDecoration(
              labelText: s.pick(
                'Birikim penceresi (ay)',
                'Accumulation window (months)',
              ),
            ),
            items: [
              for (final n in [1, 3, 6])
                DropdownMenuItem(value: n, child: Text('$n')),
            ],
            onChanged: busy ? null : (v) => setState(() => scale = v ?? 3),
          ),
          const SizedBox(height: 16),
          FilledButton(
            key: const ValueKey('simulation-run'),
            onPressed: busy || region == null ? null : run,
            child: Text(
              region == null
                  ? s.pick(
                      'Önce haritadan bölge seçin',
                      'Choose a region on the map first',
                    )
                  : s.pick('Senaryoyu hesapla', 'Calculate scenario'),
            ),
          ),
          if (busy)
            const Padding(
              padding: EdgeInsets.all(12),
              child: LinearProgressIndicator(),
            ),
          if (error != null)
            ErrorState(error: error!, compact: true, onRetry: run),
          ref
              .watch(savedSimulationsProvider)
              .when(
                loading: () => const SizedBox.shrink(),
                error: (e, _) => ErrorState(
                  error: e,
                  compact: true,
                  onRetry: () => ref.invalidate(savedSimulationsProvider),
                ),
                data: (saved) => Column(
                  children: [
                    for (final r in saved.take(10))
                      ListTile(
                        contentPadding: EdgeInsets.zero,
                        title: Text(r.name.isEmpty ? r.id : r.name),
                        subtitle: Text(r.status),
                        onTap: () => setState(() {
                          result = r;
                          comparison = null;
                        }),
                      ),
                    if (result != null &&
                        saved.any((r) => r.id != result!.id)) ...[
                      DropdownButtonFormField<String>(
                        key: ValueKey(result!.id),
                        decoration: InputDecoration(
                          labelText: s.pick(
                            'Karşılaştırılacak senaryo',
                            'Scenario to compare',
                          ),
                        ),
                        items: saved
                            .where((r) => r.id != result!.id)
                            .map(
                              (r) => DropdownMenuItem(
                                value: r.id,
                                child: Text(
                                  r.name.isEmpty ? r.id : r.name,
                                  maxLines: 1,
                                  overflow: TextOverflow.ellipsis,
                                ),
                              ),
                            )
                            .toList(),
                        onChanged: (v) => setState(() => other = v),
                      ),
                      TextButton(
                        onPressed: busy || other == null ? null : compare,
                        child: Text(s.pick('Karşılaştır', 'Compare')),
                      ),
                    ],
                  ],
                ),
              ),
        ],
        if (result != null) ...[
          const Divider(),
          Text(result!.name, style: Theme.of(context).textTheme.titleLarge),
          if (!result!.isOk)
            Text(
              result!.insufficientReason ??
                  s.pick(
                    'Bu dönem için yeterli veri yok.',
                    'Insufficient data for this period.',
                  ),
            ),
          if (result!.isOk) ...[
            for (final indicator in result!.indicators)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 8),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      '${indicator.label} (${indicator.unit})',
                      style: Theme.of(context).textTheme.titleSmall,
                    ),
                    Text(
                      '${s.pick('Referans', 'Reference')}: ${indicator.reference.toStringAsFixed(2)} → ${s.pick('Senaryo', 'Scenario')}: ${indicator.scenario.toStringAsFixed(2)}',
                    ),
                  ],
                ),
              ),
            for (final series in result!.series) ScenarioChart(series: series),
          ],
          if (result!.narrative != null) ...[
            Text(
              s.pick(
                'Yapay zekâ açıklaması — doğrulanmadı',
                'AI explanation — unverified',
              ),
              style: TextStyle(color: context.eco.warning),
            ),
            Text(result!.narrative!),
          ],
          for (final text in [...result!.assumptions, ...result!.limitations])
            Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Text(text, style: Theme.of(context).textTheme.bodySmall),
            ),
        ],
        if (comparison != null) ...[
          const Divider(),
          Text(
            s.pick('Senaryo karşılaştırması', 'Scenario comparison'),
            style: Theme.of(context).textTheme.titleLarge,
          ),
          if (comparison!['same_baseline'] != true)
            Text(
              s.pick(
                'Referans dönemleri farklı; farkları doğrudan kıyaslamayın.',
                'Reference periods differ; do not compare deltas directly.',
              ),
            ),
          for (final d in (comparison!['deltas'] as List? ?? []))
            ListTile(
              contentPadding: EdgeInsets.zero,
              title: Text('${d['label']} (${d['unit']})'),
              subtitle: Text(
                '${d['a_scenario']} → ${d['b_scenario']} · Δ ${d['delta']}',
              ),
            ),
        ],
        const SizedBox(height: 20),
        Text(
          'Weather data by Open-Meteo.com · ERA5 · CC BY 4.0',
          style: Theme.of(context).textTheme.bodySmall,
        ),
      ],
    );
  }
}

class ScenarioChart extends StatelessWidget {
  const ScenarioChart({super.key, required this.series});
  final SimulationSeries series;
  @override
  Widget build(BuildContext context) {
    if (series.points.isEmpty) return const SizedBox.shrink();
    final labels = {
      'precipitation': 'Yağış / Rainfall (mm)',
      'temperature_mean': 'Sıcaklık / Temperature (°C)',
      'pet': 'Potansiyel buharlaşma / PET (mm)',
      'water_balance': 'Su dengesi / Water balance (mm)',
      'spi': 'Yağış indeksi / SPI',
      'wbsi': 'Su dengesi indeksi / WBSI',
    };
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            series.label.isNotEmpty
                ? '${series.label} (${series.unit})'
                : labels[series.key] ?? series.key,
          ),
          Text(
            '${series.points.first.month} → ${series.points.last.month}',
            style: Theme.of(context).textTheme.bodySmall,
          ),
          Text(
            context.s.pick(
              'Mavi: referans · Yeşil: senaryo',
              'Blue: reference · Green: scenario',
            ),
          ),
          const SizedBox(height: 8),
          SizedBox(
            height: 180,
            child: LineChart(
              LineChartData(
                lineTouchData: const LineTouchData(enabled: true),
                titlesData: const FlTitlesData(
                  topTitles: AxisTitles(
                    sideTitles: SideTitles(showTitles: false),
                  ),
                  rightTitles: AxisTitles(
                    sideTitles: SideTitles(showTitles: false),
                  ),
                  bottomTitles: AxisTitles(
                    sideTitles: SideTitles(showTitles: false),
                  ),
                ),
                lineBarsData: [
                  LineChartBarData(
                    color: context.eco.info,
                    dotData: const FlDotData(show: false),
                    barWidth: 2,
                    spots: [
                      for (var i = 0; i < series.points.length; i++)
                        series.points[i].reference == null
                            ? FlSpot.nullSpot
                            : FlSpot(i.toDouble(), series.points[i].reference!),
                    ],
                  ),
                  LineChartBarData(
                    color: context.eco.green,
                    dotData: const FlDotData(show: false),
                    barWidth: 2,
                    dashArray: [5, 3],
                    spots: [
                      for (var i = 0; i < series.points.length; i++)
                        series.points[i].scenario == null
                            ? FlSpot.nullSpot
                            : FlSpot(i.toDouble(), series.points[i].scenario!),
                    ],
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}
