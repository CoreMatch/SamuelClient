import { Box, Paper, Typography } from '@mui/material';
import { CheckCircle, RadioButtonUnchecked } from '@mui/icons-material';
import { t } from '../i18n';
import { tokens } from '../theme';

// GetStartedChecklist is the Modrinth-style first-run checklist (research
// 04 §A6): three facts derived from local state — an account exists, a
// version is installed, the game has launched once. It disappears when all
// three are done and never blocks anything.

interface GetStartedChecklistProps {
  hasAccount: boolean;
  installedCount: number;
  hasLaunched: boolean;
}

export default function GetStartedChecklist({ hasAccount, installedCount, hasLaunched }: GetStartedChecklistProps) {
  const items = [
    { done: hasAccount, label: t('checklist.signIn'), hint: t('checklist.signInHint') },
    { done: installedCount > 0, label: t('checklist.installVersion'), hint: t('checklist.installVersionHint') },
    { done: hasLaunched, label: t('checklist.firstLaunch'), hint: t('checklist.firstLaunchHint') },
  ];

  if (items.every(item => item.done)) {
    return null;
  }

  return (
    <Paper sx={{ p: 2, mb: 2, backgroundColor: 'background.paper', borderRadius: tokens.radius.lg }}>
      <Typography variant="h6" sx={{ color: 'text.primary', mb: 1 }}>
        {t('checklist.title')}
      </Typography>
      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
        {items.map(item => (
          <Box key={item.label} sx={{ display: 'flex', gap: 1.5, alignItems: 'flex-start' }}>
            {item.done ? (
              <CheckCircle sx={{ fontSize: 20, color: tokens.color.success, mt: 0.25 }} />
            ) : (
              <RadioButtonUnchecked sx={{ fontSize: 20, color: 'text.secondary', mt: 0.25 }} />
            )}
            <Box>
              <Typography variant="body2" sx={{ color: item.done ? 'text.secondary' : 'text.primary', textDecoration: item.done ? 'line-through' : 'none' }}>
                {item.label}
              </Typography>
              {!item.done && (
                <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                  {item.hint}
                </Typography>
              )}
            </Box>
          </Box>
        ))}
      </Box>
    </Paper>
  );
}
