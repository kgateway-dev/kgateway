//go:generate -command asset go run asset.go
//go:generate asset afl_3.0.txt
//go:generate asset agpl_3.0.txt
//go:generate asset apache_2.0.txt
//go:generate asset artistic_2.0.txt
//go:generate asset bsd_2_clause.txt
//go:generate asset bsd_3_clause_clear.txt
//go:generate asset bsd_3_clause.txt
//go:generate asset cc0_1.0.txt
//go:generate asset epl_1.0.txt
//go:generate asset gpl_2.0.txt
//go:generate asset gpl_3.0.txt
//go:generate asset isc.txt
//go:generate asset lgpl_2.1.txt
//go:generate asset lgpl_3.0.txt
//go:generate asset mit.txt
//go:generate asset mpl_2.0.txt
//go:generate asset ms_pl.txt
//go:generate asset ms_rl.txt
//go:generate asset no_license.txt
//go:generate asset ofl_1.1.txt
//go:generate asset osl_3.0.txt
//go:generate asset unlicense.txt
//go:generate asset wtfpl.txt

package assets

var (
	Assets = []asset{}
)

func txt(a asset) asset {
	Assets = append(Assets, a)
	return a
}
