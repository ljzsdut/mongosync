package main

import (
	"flag"
	"fmt"
	"log"
	"mongosync/utils"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/fatih/set.v0"
)

func main() {

	// 1、对于已经存在的索引的异常捕获处理
	// 使用--oplog参数，强烈不建议使用nsFrom_To参数和dbFrom_To 参数. TODO:考虑使用clone函数进行重放完成后，先克隆然后删除旧集合

	/*
		mongosync  基于快照模式的同步
		mongosync -h
		mongoysnc --src_host "HOST" --src_port "PORT" --dst_host "HOST"  --dst_port "PORT"
		mongosync  --db "GlobalDB,CUST_U_TEST"  //只对GlobalDB和CUST_U_TEST这两个库进行数据同步。--db缺省表示对实例中的除了admin和local之外所有库进行同步
		mongosync  --oplog 基于增量模式的实时同步(推荐，但要求src开启oplog)
		mongosync  --sync_oplog 基于增量模式的实时同步(要求src开启oplog)，但是oplog不会进行重放，会将oplog存放在des实例中的syncoplog.oplog.rs集合中，需要使用--replayoplog参数进行手动重放。一般用于--oplog模式下，源oplog过期失效的情况。
		mongosync  --db "GlobalDB,CUST_U_TEST"  --oplog // --oplog使用基于oplog的实时同步
		mongosync  --db "GlobalDB,CUST_U_TEST"  --nsExclude "CUST_U_TEST.files.file,CUST_U_TEST.files.chunks"  // 对GlobalDB,CUST_U_TEST库中的所有集合进行同步，但是排除CUST_U_TEST.files.file,CUST_U_TEST.files.chunks集合
		mongosync  --db "GlobalDB,CUST_U_TEST"  --nsInclude "CUST_U_TEST.files.file,CUST_U_TEST.files.chunks"  // 仅对CUST_U_TEST.files.file,CUST_U_TEST.files.chunks集合进行同步，这种情况可以省略--db参数
		mongosync --replayoplog [--src_op_ns "syncoplog.oplog.rs"] --op_start arg [--op_end arg ] [--db arg ，--nsExclude|nsInclude arg ,--dbFrom_To arg ,--nsFrom_To arg] // 手动进行oplog重放
		mongosync --syncoplog
		mongosync --overwrite  对于"_id"已经存在的数据，采用覆盖的方式还是采用跳过的方式，默认跳过。
		mongosync --no_index 是否创建索引，如果索引已经存在，再创建会失败
		mongosync --threadNum arg 指定进行通过的线程数量，默认是20个线程。可以用来控制流量
		mongosync --dbFrom_To arg 数据库名称映射（这些db必须存在于-db参数列表中）
		mongosync --nsFrom_To arg 名称空间映射（这些db必须存在于-db参数列表中）

	*/

	var (
		src_host, src_user, src_passwd, src_auth_db    string
		src_port                                       int
		dst_host, dst_user, dst_passwd, dst_auth_db    string
		dst_port                                       int
		oplog, sync_oplog, replayoplog                 bool
		db, nsExclude, nsInclude, dbFrom_To, nsFrom_To string
		op_start, op_end, src_op_ns                    string
		overwrite, no_index                            bool
		threadNum                                      int
	)

	// 连接mongodb相关参数
	flag.StringVar(&src_host, "sh", "0.0.0.0", "the source mongodb server's ip")
	flag.IntVar(&src_port, "sP", 27017, "the source mongodb server's port")
	flag.StringVar(&src_user, "su", "", "the source mongodb server's logging user")
	flag.StringVar(&src_passwd, "sp", "", "the source mongodb server's logging password")
	flag.StringVar(&src_auth_db, "sd", "", "the source mongodb server's auth db")

	flag.StringVar(&dst_host, "dh", "", "the destination mongodb server's ip")
	flag.IntVar(&dst_port, "dP", 27017, "the destination mongodb server's port")
	flag.StringVar(&dst_user, "du", "", "the destination mongodb server's logging user")
	flag.StringVar(&dst_passwd, "dp", "", "the destination mongodb server's logging password")
	flag.StringVar(&dst_auth_db, "dd", "", "the destination mongodb server's auth db")

	// 是否启用oplog进行增量同步；是否将oplog同步到目标mongodb实例中；oplog和sync_oplog互斥
	flag.BoolVar(&oplog, "oplog", false, "whether to enable oplog for incremental synchronization")
	flag.BoolVar(&sync_oplog, "sync_oplog", false, "whether to synchronize oplog to the destination mongodb")

	// 名称空间过滤及映射相关参数,生效顺序：db>nsExclude、nsInclude>dbFrom_To>nsFrom_To
	flag.StringVar(&db, "db", "", "databases to sync.Default for all dbs in instance. Format:<database-name,...>. Namespace control sub-parameters: --nsExclude,--nsInclude,--nsFrom_To")
	flag.StringVar(&nsExclude, "nsExclude", "", "exclude matching namespaces. Format:<namespace,...>")
	flag.StringVar(&nsInclude, "nsInclude", "", "include matching namespaces. Format:<namespace,...>")
	flag.StringVar(&dbFrom_To, "dbFrom_To", "", "rename matching databasename. Format:<src_dbname:dst_dbname,...>")
	flag.StringVar(&nsFrom_To, "nsFrom_To", "", "rename matching namespaces. Format:<src_namespace:dst_namespace,...>")

	// oplog的replay操作参数
	flag.BoolVar(&replayoplog, "replayoplog", false, "repaly oplog,must have matching op_start")
	flag.StringVar(&op_start, "op_start", "0,0", "the start timestamp to sync oplog. Format:<\"m,n\">")
	flag.StringVar(&op_end, "op_end", "0,0", "the end timestamp to sync oplog,the default value of \"0,0\" indicates the current latest oplog. Format:<\"m,n\">")
	flag.StringVar(&src_op_ns, "src_op_ns", "local.oplog.rs", "the namespace of the source of oplog. Format:<namespace,...>")
	// 其他TODO参数
	flag.BoolVar(&no_index, "no_index", false, "whether to clone the db or collection corresponding index")
	flag.IntVar(&threadNum, "threadNum", 20, "Number of threads performing collection synchronization")
	flag.BoolVar(&overwrite, "overwrite", false, "whether to overwrite documents whose \"_id\" field already exists")
	// flag.StringVar(&query, "query", "", "query filter, as a JSON string, e.g., '{x:{$gt:1}}'") // TODO

	flag.Parse()

	if dst_host == "" {
		fmt.Println("未指定--dst_host参数，请使用合理的参数:")
		flag.Usage()
		os.Exit(1)
	}

	if nsExclude != "" && nsInclude != "" {
		log.Fatalln("--nsExclude与--nsInclude参数互斥，不能同时使用")
	}
	if oplog != false && sync_oplog != false {
		log.Fatalln("--oplog与--sync_oplog参数互斥，不能同时使用")
	}


	src := utils.NewMongoArgs()
	src.SetHost(src_host)
	src.SetPort(src_port)
	src.SetUsername(src_user)
	src.SetPassword(src_passwd)
	src.SetAuthenticationDatabase(src_auth_db)

	dst := utils.NewMongoArgs()
	dst.SetHost(dst_host)
	dst.SetPort(dst_port)
	dst.SetUsername(dst_user)
	dst.SetPassword(dst_passwd)
	dst.SetAuthenticationDatabase(dst_auth_db)

	// 使用--oplog或--sync_oplog参数时：在所有连接src库进行操作之前，获取当前最新的oplog对应的timestamp
	var (
		start_ts, end_ts primitive.Timestamp
		err              error
	)
	if sync_oplog || oplog {
		start_ts, err = utils.CustGetLatestOplogTimestamp(src)  //该函数执行需要访问admin库
		if err != nil {
			log.Fatalln("获取当前最新的oplog对应的timestamp失败,请确认用户是否可以访问admin库(src)：", err)
		}
	}

	//--------------------------------------------------------------------------------------------
	// 分析db列表 ：dbSlice
	var (
		dbSlice []string // dbSlice是<最终>要同步的db切片
		nsSlice []string // nsSlice是<最终>要同步的ns切片
	)
	if db != "" {   // db参数的的格式：<database-name,...>
		srcAllDbs := utils.CustGetDbs(src)
		cmdDbs := strings.Split(db, ",")
		srcAllDbsSet := set.New(set.ThreadSafe)
		for _, SrcDb := range srcAllDbs {
			srcAllDbsSet.Add(SrcDb)
		}
		cmdDbsSet := set.New(set.ThreadSafe)
		for _, SrcDb := range cmdDbs {
			cmdDbsSet.Add(SrcDb)
		}
		dbSlice = set.StringSlice(set.Intersection(srcAllDbsSet, cmdDbsSet)) // 交集
	} else {
		dbSlice = utils.CustGetDbs(src) // dbSlice=srcAllDbs
	}
	// dbSlice是要同步的db切片
	//--------------------------------------------------------------------------------------------

	// 使用集合操作进行nsInclude和nsExclude参数过滤：nsSlice
	allNsSet := set.New(set.ThreadSafe)  // 未经过nsInclude和nsExclude参数过滤的所有的ns,放在集合allNsSet中
	taskNsSet := set.New(set.ThreadSafe) // 经过nsInclude和nsExclude参数过滤的所有的ns,放在集合taskNsSet中

	// 将dbSlicce转换为ns格式的集合－－>allNsSet
	for _, SrcDb := range dbSlice {
		for _, SrcColl := range utils.CustGetColls(src, SrcDb) {
			allNsSet.Add(fmt.Sprintf("%s.%s", SrcDb, SrcColl))
		}
	}

	if nsExclude != "" { // 对allNsSet使用--nsInclude和--nsExclude参数进行过滤过滤，最终有效ns放在nsSlice这个切片中。
		nsExcludeSet := set.New(set.ThreadSafe)
		for _, ns := range strings.Split(nsExclude, ",") {
			nsExcludeSet.Add(ns)
		}
		taskNsSet = set.Difference(allNsSet, nsExcludeSet) // 差集
	} else if nsInclude != "" {  // --nsExclude参数和--nsInclude参数互斥
		nsIncludeSet := set.New(set.ThreadSafe)
		for _, ns := range strings.Split(nsInclude, ",") {
			nsIncludeSet.Add(ns)
		}
		taskNsSet = set.Intersection(allNsSet, nsIncludeSet) // 交集
	} else {
		taskNsSet = allNsSet
	}
	nsSlice = set.StringSlice(taskNsSet) // 元素格式为： db.coll
	sort.Strings(nsSlice)
	// nsSlice是要同步的ns切片
	//--------------------------------------------------------------------------------------------

	// --nsFrom_To、--dbFrom_To参数处理
	nsnsMap := make(map[string]string) // nsnsMap是要ns映射的字典，元素形如：dbFrom.[coll|$cmd]:dbTo.[coll|$cmd}
	var errmaps []string
	// 只将dbname进行映射，collname保持不变，保存为nsnsMap
	if dbFrom_To != "" {  // Format:<src_dbname:dst_dbname,...>
		for _, dbmap := range strings.Split(dbFrom_To, ",") {
			reg := regexp.MustCompile(`^([^:]+)\:([^:]+)$`)  // 正则字符串：非冒号开头和结尾，但是中间必须有冒号
			if reg.MatchString(dbmap) {
				dbFrom := strings.SplitN(dbmap, ":", 2)[0]
				dbTo := strings.SplitN(dbmap, ":", 2)[1]
				for _, coll := range utils.CustGetColls(src, dbFrom) {
					nsnsMap[fmt.Sprintf("%s.%s", dbFrom, coll)] = fmt.Sprintf("%s.%s", dbTo, coll)
					nsnsMap[fmt.Sprintf("%s.$cmd", dbFrom)] = fmt.Sprintf("%s.$cmd", dbTo)
				}
			} else {
				errmaps = append(errmaps, dbmap)
			}
		}
		if len(errmaps) > 0 {
			log.Fatalln("--dbFrom_To参数格式错误：", errmaps)
		}
	}

	// 只将dbname进行映射，collname保持不变，保存为nsnsMap
	if nsFrom_To != "" {  // Format:<src_namespace:dst_namespace,...>
		for _, nsmap := range strings.Split(nsFrom_To, ",") {
			reg := regexp.MustCompile(`^([^.:]+)\.([^:]+)\:([^.:]+)\.([^:]+)$`)
			if reg.MatchString(nsmap) {
				key := strings.SplitN(nsmap, ":", 2)[0]
				value := strings.SplitN(nsmap, ":", 2)[1]
				nsnsMap[key] = value   // 对于已经存在的key,进行跟新；如果不存在，直接创建
			} else {
				errmaps = append(errmaps, nsmap)
			}
		}
		if len(errmaps) > 0 {
			log.Fatalln("--nsFrom_To参数格式错误：", errmaps)
		}
	}
	// nsnsMap是要ns映射的字典。表示需要进行转换的的ns

	//-------------------------------------------------------------------------------------------
	// 将nsSlice中的ns转换成utils.NsMap结构体
	var nsStructSlice []*utils.NsMap
	for _, ns := range nsSlice { // ns格式：db.coll
		nsStructSlice = append(nsStructSlice, utils.CustFilter(ns, nsnsMap))
	}
	// nsStructSlice是最终要进行操作的对象

	fmt.Println("即将对以下集合进行操作：")
	for _, task := range nsStructSlice {
		fmt.Printf("源:%-60s目标:%-s\n", fmt.Sprintf("%s.%s", task.SrcDb, task.SrcColl), fmt.Sprintf("%s.%s", task.DstDb, task.DstColl))
	}

	var answer string
label:
	fmt.Print("请确认以上信息是否正确，输入[yes|YES]继续，输入[no|NO]退出：")
	fmt.Scanln(&answer)
	if "YES" == strings.TrimSpace(answer) || "yes" == strings.TrimSpace(answer) {
		//continue
	} else if "NO" == strings.TrimSpace(answer) || "no" == strings.TrimSpace(answer) {
		os.Exit(1)
	} else {
		goto label
	}

	//-------------------------------------------------------------------------------------------
	if !replayoplog {
		// 生产者，不断地将nsStructSlice中的元素放入nsQueue
		var nsQueue = make(chan *utils.NsMap, 20)
		go func(taskQueue chan *utils.NsMap, nsStructSlice []*utils.NsMap) {
			for _, nsmap := range nsStructSlice {
				nsQueue <- nsmap
			}
			close(taskQueue)
		}(nsQueue, nsStructSlice)

		//消费者：不断地从nsQueue中获取task来运行CustSyncCollection函数，直到nsQueue关闭
		worker := func(wg *sync.WaitGroup) {
			defer wg.Done()
			for NSMAP := range nsQueue {
				utils.CustSyncCollection(src, NSMAP.SrcDb, NSMAP.SrcColl, dst, NSMAP.DstDb, NSMAP.DstColl, overwrite, no_index)
			}
		}

		// 协程池
		var wg sync.WaitGroup
		for i := 0; i < threadNum; i++ {
			wg.Add(1)
			go worker(&wg)
		}
		wg.Wait()
		log.Println("基于快照的集合同步完成...")

		if sync_oplog == true {
			log.Println("开始进行oplog同步至目标mongodb实例...")
			fmt.Printf("请使用--replayoplog --src_op_ns \"syncoplog.oplog.rs\" --op_start \"%d,%d\" 等参数进行oplog重放\n", start_ts.T, start_ts.I)
			go utils.CustSyncOplog(src, dst, start_ts)
			// 捕获ctrl+c，进行--replayoplog相关参数的提示并退出sync_oplog操作
			func() {
				c := make(chan os.Signal, 1)
				signal.Notify(c, os.Interrupt) //signal包不会为了向c发送信息而阻塞（就是说如果发送时c阻塞了，signal包会直接放弃）.调用者应该保证c有足够的缓存空间可以跟上期望的信号频率。对使用单一信号用于通知的通道，缓存为1就足够了。
				<-c                            // Block until a signal is received.
				fmt.Printf("请使用--replayoplog --src_op_ns \"syncoplog.oplog.rs\" --op_start \"%d,%d\" 等参数进行oplog重放\n", start_ts.T, start_ts.I)
				os.Exit(1)
			}()
		} else if oplog {
			log.Println("开始进行oplog重放...")
			utils.CustReplayOplog(src, dst, start_ts, end_ts, "local.oplog.rs", nsSlice, nsnsMap)
		}
	} else {
		// 获取start_ts
		if op_start == "0,0" {
			log.Fatalln("--op_start为必选参数，请正确指定--op_start参数")
		} else {
			T, err := strconv.Atoi(strings.SplitN(op_start, ",", 2)[0])
			if err != nil {
				log.Fatalln("--op_start格式有误")
			}
			I, err := strconv.Atoi(strings.SplitN(op_start, ",", 2)[1])
			if err != nil {
				log.Fatalln("--op_start格式有误")
			}
			start_ts = primitive.Timestamp{uint32(T), uint32(I)}
		}
		// end_ts
		T, err := strconv.Atoi(strings.SplitN(op_end, ",", 2)[0])
		if err != nil {
			log.Fatalln("--op_end格式有误")
		}
		I, err := strconv.Atoi(strings.SplitN(op_end, ",", 2)[1])
		if err != nil {
			log.Fatalln("--op_end格式有误")
		}
		end_ts = primitive.Timestamp{uint32(T), uint32(I)}

		utils.CustReplayOplog(src, dst, start_ts, end_ts, src_op_ns, nsSlice, nsnsMap)
		log.Println("oplog重放完毕，如果需要，请手动删除dst实例中的syncoplog库！")
		// defer 删除syncoplog库
	}
}
