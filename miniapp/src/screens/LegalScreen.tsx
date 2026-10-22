import { useNavigate, useParams } from 'react-router-dom';
import { BackButton } from '@/telegram/hooks';
import { LEGAL_DOCS, type LegalDocId } from '@/lib/legal';
import { useT } from '@/i18n';
import { AppHeader, Card, Icon, Screen } from '@/components/ui';

const DOCS: LegalDocId[] = ['offer', 'personal', 'privacy'];

/** S13 · Legal documents - readable inside Telegram, no HTML injection. */
export function LegalScreen() {
  const navigate = useNavigate();
  const t = useT();
  const { doc } = useParams<{ doc?: string }>();
  const id = (DOCS.includes(doc as LegalDocId) ? doc : 'offer') as LegalDocId;
  const document = LEGAL_DOCS[id];

  return (
    <Screen header={<AppHeader title={t('Документы')} onBack={() => navigate(-1)} />}>
      <div className="flex flex-col gap-3 pt-2">
        <div className="flex gap-2 overflow-x-auto px-1 pb-1">
          {DOCS.map((code) => (
            <button
              key={code}
              type="button"
              onClick={() => navigate(`/legal/${code}`, { replace: true })}
              className={
                code === id
                  ? 'h-8 shrink-0 rounded-full bg-accent-soft px-3.5 text-caption font-semibold text-accent'
                  : 'h-8 shrink-0 rounded-full bg-neutral-soft px-3.5 text-caption text-ink-2'
              }
            >
              {t(LEGAL_DOCS[code].title.split(' ').slice(0, 2).join(' '))}
            </button>
          ))}
        </div>

        <span className="px-1 text-headline-md text-ink">{document.title}</span>
        <span className="px-1 text-caption text-ink-2">{document.updated}</span>

        <Card className="gap-4 p-4">
          {document.sections.map((section) => (
            <div key={section.heading} className="flex flex-col gap-1">
              <span className="text-headline-sm text-ink">{section.heading}</span>
              <p className="text-body-lg text-ink-2">{section.text}</p>
            </div>
          ))}
        </Card>

        <div className="flex flex-col items-center gap-1 pb-4 pt-2 text-center">
          <Icon name="verified" size={20} className="text-success" filled />
          <span className="text-caption text-ink-2">
            {t('Вопросы по документам - в поддержке')}
          </span>
        </div>
      </div>
      <BackButton onClick={() => navigate(-1)} />
    </Screen>
  );
}
