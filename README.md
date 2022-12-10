# Valutazione MIGP
Questa repository contiene il codice utilizzato per valutare le performance del protocollo MIGP. Si tratta della libreria scritta in Go do [Cloudflare](https://github.com/cloudflare/migp-go), modificata e adattata alle necessità del lavoro di tesi.

## Istruzioni per riprodurre i risultati ottenuti
Per effettuare dei test è necessario fornire delle credenziali nel formato username:password. Poiché la collezione di credenziali utilizzata appartiene a data breach reali, è necessario inviare una richiesta all'indirizzo 257242@studenti.unimore.it per ottenere accesso a tali credenziali.
### Build

	mkdir -p bin && go build -o bin/ ./cmd/...

### Configurazione

Per poter utilizzare le credenziali elaborate nella fase di pre-processing, è necessario salvare la configurazione utilizzata e caricarla ad ogni avvio del server. 

Salvataggio configurazione:

    bin/server -dump-config > config.json

Caricamento configurazione:

    bin/server -config config.json

È possibile modificare la configuraizone salvata per cambiare parametri come la lunghezza del bucketID oppure lo slow hashing.

### Pre-processing
    bin/server -config config.json -num-variants numero_di_varianti -indir nome_directory
nome_directory è la directory contenente le credenziali.
### Test pre-processing
Per ottenere informazioni sui bucket generati utilizzare il comando seguente:

    bin/server -config config.json -test

### Test online-computation
Avviare il server:
    
    bin/server -config config.json -start

Avviare il client passando un file contenente le query nel formato username:password oppure passarle direttamente tramite stdin:

    bin/client -infile nome_file