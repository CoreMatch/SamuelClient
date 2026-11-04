
import { useState } from 'react';
import { Box, Typography, Button, Card, CardContent, CircularProgress, Paper } from '@mui/material';
import { GetDBSchema } from '../../wailsjs/go/main/App';

interface ApiResponse {
  data: any;
  success: boolean;
  error?: string;
  time?: number;
}

// DebugView surfaces internal diagnostics. ADR-0004: the previous
// health-check and ping panels exercised the local HTTP API, which has been
// retired in favor of Wails bindings; only the database schema view remains.
export default function DebugView() {
  const [schemaResult, setSchemaResult] = useState<ApiResponse | null>(null);
  const [loading, setLoading] = useState<{ schema: boolean }>({ schema: false });

  const fetchSchema = async () => {
    setLoading({ schema: true });
    try {
      const response = await GetDBSchema();
      setSchemaResult({
        data: response.data,
        success: response.success,
        error: response.error,
        time: Number(response.time),
      });
    } catch (error) {
      setSchemaResult({
        data: null,
        success: false,
        error: error instanceof Error ? error.message : 'Unknown error',
      });
    } finally {
      setLoading({ schema: false });
    }
  };

  return (
    <Box sx={{ maxWidth: 900 }}>
      <Typography variant="h4" component="h1" gutterBottom sx={{ color: 'white' }}>
        Debug & Diagnostics
      </Typography>

      <Card sx={{ backgroundColor: '#212529' }}>
        <CardContent>
          <Typography variant="h5" gutterBottom sx={{ color: '#fff' }}>
            Database Schema
          </Typography>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 2 }}>
            <Button
              variant="contained"
              onClick={fetchSchema}
              disabled={loading.schema}
              sx={{ backgroundColor: '#9c27b0', '&:hover': { backgroundColor: '#7b1fa2' } }}
            >
              {loading.schema ? <CircularProgress size={20} /> : 'Display SQL Schema'}
            </Button>
          </Box>
          {schemaResult && (
            <Paper sx={{ p: 2, backgroundColor: '#2d3137', maxHeight: 400, overflowY: 'auto' }}>
              <Typography variant="subtitle1" sx={{ color: schemaResult.success ? '#4CAF50' : '#f44336' }}>
                {schemaResult.success ? 'Success' : 'Failed'}
                {schemaResult.time && ` (${schemaResult.time}ms)`}
              </Typography>
              {schemaResult.error && (
                <Typography variant="body2" sx={{ color: '#f44336', mt: 1 }}>
                  {schemaResult.error}
                </Typography>
              )}
              {schemaResult.data && (
                <Box sx={{ mt: 2 }}>
                  {Array.isArray(schemaResult.data) ? (
                    schemaResult.data.map((item: any, index: number) => (
                      <Paper key={index} sx={{ p: 2, mb: 2, backgroundColor: '#1a1d21' }}>
                        <Typography variant="subtitle2" sx={{ color: '#ff9800', mb: 1 }}>
                          {item.name} ({item.type})
                        </Typography>
                        <Typography component="pre" sx={{ color: '#fff', whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                          {item.sql}
                        </Typography>
                      </Paper>
                    ))
                  ) : (
                    <Typography component="pre" sx={{ color: '#fff', whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                      {JSON.stringify(schemaResult.data, null, 2)}
                    </Typography>
                  )}
                </Box>
              )}
            </Paper>
          )}
        </CardContent>
      </Card>
    </Box>
  );
}
