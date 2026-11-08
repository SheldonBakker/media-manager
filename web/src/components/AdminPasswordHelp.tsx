import { DOCS_URL } from '../docs'
import { t, tx } from '../i18n'

// How a password Mediarium keeps itself is set again: by an administrator in
// Settings, or with a command on the machine that runs it. Shown under
// "Forgot your password?" on the sign-in page. emailReset is set where the
// emailed code resets the other accounts, so the text says why this one differs.
export default function AdminPasswordHelp({ emailReset = false }: { emailReset?: boolean }) {
  return (
    <>
      <p>
        {emailReset
          ? t("An administrator account keeps its password in Mediarium, so an emailed code can't reset it. Ask another administrator to set a new password in Settings > Account & Access > All accounts. If you're the only administrator, run this on the machine that runs Mediarium (swap 1000:1000 for your own PUID:PGID):")
          : t("Mediarium can't send reset emails. Ask an administrator to set a new password in Settings > Account & Access > All accounts. If you're the only administrator, run this on the machine that runs Mediarium (swap 1000:1000 for your own PUID:PGID):")}
      </p>
      <code className="forgot-cmd">docker exec -it -u 1000:1000 mediarium /app/app reset-password &lt;username&gt;</code>
      <p>
        {tx('It asks for a new password and signs that account out everywhere. Not using Docker? See {link}.', {
          link: (
            <a href={`${DOCS_URL}/accounts.md`} target="_blank" rel="noreferrer">
              {t('the accounts page')}
            </a>
          ),
        })}
      </p>
    </>
  )
}
