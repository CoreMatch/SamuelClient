import { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
  IconButton,
  Button,
  Switch,
  Chip,
} from '@mui/material';
import {
  Add,
  Delete,
  FolderOpen,
  Extension,
} from '@mui/icons-material';
import { EmptyState } from './states';
import { t } from '../i18n';
import { tokens } from '../theme';

interface Mod {
  id: string;
  name: string;
  version: string;
  mcVersion: string;
  loaderType: 'fabric' | 'forge' | 'universal';
  enabled: boolean;
  filePath?: string;
}

interface ModManagerProps {
  onOpenModsFolder: () => void;
}

const STORAGE_KEY = 'installed_mods';

export default function ModManager({ onOpenModsFolder }: ModManagerProps) {
  const [mods, setMods] = useState<Mod[]>([]);

  useEffect(() => {
    loadMods();
  }, []);

  const loadMods = () => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored) {
        setMods(JSON.parse(stored));
      }
    } catch (err) {
      console.error('Failed to load mods:', err);
    }
  };

  const saveMods = (newMods: Mod[]) => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(newMods));
      setMods(newMods);
    } catch (err) {
      console.error('Failed to save mods:', err);
    }
  };

  const handleToggleMod = (modId: string) => {
    const updated = mods.map(mod =>
      mod.id === modId ? { ...mod, enabled: !mod.enabled } : mod
    );
    saveMods(updated);
  };

  const handleDeleteMod = (modId: string) => {
    const updated = mods.filter(mod => mod.id !== modId);
    saveMods(updated);
  };

  const handleAddMod = () => {
    // Create a sample mod entry (in real app, this would open a file picker)
    const newMod: Mod = {
      id: Date.now().toString(),
      name: `Sample Mod ${mods.length + 1}`,
      version: '1.0.0',
      mcVersion: '1.20.4',
      loaderType: 'fabric',
      enabled: true,
    };
    saveMods([...mods, newMod]);
  };

  // Loader chip colors come from the shared semantic palette.
  const getLoaderColor = (loader: string) => {
    switch (loader) {
      case 'fabric':
        return tokens.color.warning;
      case 'forge':
        return tokens.color.info;
      default:
        return 'rgba(255, 255, 255, 0.24)';
    }
  };

  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Box sx={{ display: 'flex', gap: 1 }}>
          <Button
            variant="outlined"
            startIcon={<FolderOpen />}
            onClick={onOpenModsFolder}
            sx={{ borderColor: tokens.color.divider, color: 'text.secondary' }}
          >
            Open Mods Folder
          </Button>
          <Button variant="contained" startIcon={<Add />} onClick={handleAddMod}>
            Add Mod
          </Button>
        </Box>
      </Box>

      <Typography variant="body2" sx={{ color: 'text.secondary', mb: 2 }}>
        Enable or disable mods. Mods must be placed in the mods folder to work.
      </Typography>

      {mods.length === 0 ? (
        <EmptyState
          icon={<Extension sx={{ fontSize: 64, mb: 2, opacity: 0.5, color: 'text.secondary' }} />}
          title={t('mods.emptyTitle')}
          hint={t('mods.emptyHint')}
        />
      ) : (
        <List sx={{ backgroundColor: 'background.paper', borderRadius: tokens.radius.lg }}>
          {mods.map((mod) => (
            <ListItem
              key={mod.id}
              sx={{
                '&:hover': { backgroundColor: 'rgba(255, 255, 255, 0.06)' },
              }}
            >
              <ListItemText
                primary={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Typography sx={{ color: 'text.primary' }}>{mod.name}</Typography>
                    <Chip
                      size="small"
                      label={mod.version}
                      sx={{
                        height: 20,
                        fontSize: '0.7rem',
                        backgroundColor: 'rgba(255, 255, 255, 0.12)',
                        color: 'text.secondary',
                      }}
                    />
                  </Box>
                }
                secondary={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 0.5 }}>
                    <Chip
                      size="small"
                      label={`MC ${mod.mcVersion}`}
                      sx={{
                        height: 20,
                        fontSize: '0.7rem',
                        backgroundColor: 'rgba(255, 255, 255, 0.12)',
                        color: 'text.secondary',
                      }}
                    />
                    <Chip
                      size="small"
                      label={mod.loaderType}
                      sx={{
                        height: 20,
                        fontSize: '0.7rem',
                        backgroundColor: getLoaderColor(mod.loaderType),
                        color: tokens.color.deep,
                      }}
                    />
                  </Box>
                }
              />
              <ListItemSecondaryAction>
                <Switch
                  edge="end"
                  checked={mod.enabled}
                  onChange={() => handleToggleMod(mod.id)}
                  color="primary"
                />
                <IconButton
                  edge="end"
                  onClick={() => handleDeleteMod(mod.id)}
                  sx={{ color: tokens.color.danger, ml: 1 }}
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
