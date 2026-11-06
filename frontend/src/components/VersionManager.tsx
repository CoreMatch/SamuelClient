import { useState, useEffect, useCallback } from 'react';
import {
  Box,
  Typography,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
  IconButton,
  Chip,
  Tab,
  Tabs,
} from '@mui/material';
import { CheckCircle, CloudDownload, Delete } from '@mui/icons-material';
import { InstalledVersion } from '../types';
import InstallWizard from './InstallWizard';
import { EmptyState } from './states';
import { t } from '../i18n';
import { tokens } from '../theme';

interface VersionManagerProps {
  onSelectVersion: (version: InstalledVersion) => void;
}

const STORAGE_KEY = 'installed_versions';

// VersionManager hosts the three-step install wizard (research 03 §8.3)
// and the installed-version list. Installations run through the wizard;
// this component owns the localStorage working copy of installed versions.
export default function VersionManager({ onSelectVersion }: VersionManagerProps) {
  const [installedVersions, setInstalledVersions] = useState<InstalledVersion[]>([]);
  const [tabValue, setTabValue] = useState(0);

  const loadInstalledVersions = useCallback(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored) {
        setInstalledVersions(JSON.parse(stored));
      }
    } catch (err) {
      console.error('Failed to load installed versions:', err);
    }
  }, []);

  useEffect(() => {
    loadInstalledVersions();
  }, [loadInstalledVersions]);

  const handleDelete = (versionId: string) => {
    const updated = installedVersions.filter(v => v.id !== versionId);
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(updated));
    } catch (err) {
      console.error('Failed to save installed versions:', err);
    }
    setInstalledVersions(updated);
  };

  // Version-type chip colors come from the shared semantic palette.
  const getVersionTypeColor = (type: string) => {
    switch (type) {
      case 'release':
        return tokens.color.success;
      case 'snapshot':
        return tokens.color.warning;
      case 'old_beta':
        return tokens.color.info;
      default:
        return 'rgba(255, 255, 255, 0.24)';
    }
  };

  return (
    <Box>
      <Tabs
        value={tabValue}
        onChange={(_, newValue) => setTabValue(newValue)}
        sx={{
          mb: 2,
          '& .MuiTab-root': { color: 'text.secondary' },
          '& .Mui-selected': { color: tokens.color.accent },
          '& .MuiTabs-indicator': { backgroundColor: tokens.color.accent },
        }}
      >
        <Tab label={t('versions.tab.install')} />
        <Tab label={t('versions.tab.installed')} />
      </Tabs>

      {tabValue === 0 ? (
        <InstallWizard onInstalled={loadInstalledVersions} />
      ) : installedVersions.length === 0 ? (
        <EmptyState
          icon={<CloudDownload sx={{ fontSize: 64, mb: 2, opacity: 0.5, color: 'text.secondary' }} />}
          title={t('versions.emptyTitle')}
          hint={t('versions.emptyHint')}
          actionLabel={t('versions.browse')}
          onAction={() => setTabValue(0)}
        />
      ) : (
        <List sx={{ backgroundColor: 'background.paper', borderRadius: tokens.radius.lg }}>
          {installedVersions.map((version) => (
            <ListItem
              key={version.id}
              sx={{
                cursor: 'pointer',
                '&:hover': { backgroundColor: 'rgba(255, 255, 255, 0.06)' },
              }}
              onClick={() => onSelectVersion(version)}
            >
              <ListItemText
                primary={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Typography sx={{ color: 'text.primary' }}>{version.id}</Typography>
                    <CheckCircle sx={{ fontSize: 16, color: tokens.color.success }} />
                  </Box>
                }
                secondary={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 0.5 }}>
                    <Chip
                      size="small"
                      label={version.type}
                      sx={{
                        height: 20,
                        fontSize: '0.7rem',
                        backgroundColor: getVersionTypeColor(version.type),
                        color: tokens.color.deep,
                      }}
                    />
                    {version.loaderType && (
                      <Chip
                        size="small"
                        label={`${version.loaderType} ${version.loaderVersion || ''}`}
                        sx={{
                          height: 20,
                          fontSize: '0.7rem',
                          backgroundColor: 'rgba(255, 255, 255, 0.12)',
                          color: 'text.secondary',
                        }}
                      />
                    )}
                  </Box>
                }
              />
              <ListItemSecondaryAction>
                <IconButton
                  edge="end"
                  onClick={(e) => {
                    e.stopPropagation();
                    handleDelete(version.id);
                  }}
                  sx={{ color: tokens.color.danger }}
                >
                  <Delete />
                </IconButton>
              </ListItemSecondaryAction>
            </ListItem>
          ))}
        </List>
      )}
    </Box>
  );
}
