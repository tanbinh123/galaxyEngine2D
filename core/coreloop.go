package core

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"galaxyzeta.io/engine/base"
	"galaxyzeta.io/engine/ecs/component"
	"galaxyzeta.io/engine/graphics"

	cc "galaxyzeta.io/engine/infra/concurrency"
	"galaxyzeta.io/engine/linalg"
)

// InstantiateFunc receives an IGameObject2D constructor.
type InstantiateFunc func() base.IGameObject2D
type gameLoopStats int8

const InstantiateChannelSize = 256
const DeconstructionChannelSize = 256
const Debug = false
const (
	GameLoopStats_Created = iota
	GameLoopStats_Initialized
	GameLoopStats_Running
)

// AppConfig stores all user defined configs.
type AppConfig struct {
	Resolution  *linalg.Vector2f64
	PhysicalFps int
	RenderFps   int
	Parallelism int
	Title       string
	RenderFunc  func() // will be called in openGL loop.
	InitFunc    func() // will be called before we start main thread.
}

// Application communicates with OpenGL frontend to do rendering jobs, also manages all sub routines for physical updates.
// There is only one Application in each process.
type Application struct {
	// --- basic
	initFunc func()        // InitFunc will be called at the very beginning of the game. Recommend to do some pre-resource loading work here.
	status   gameLoopStats // Describes the working status of current gameLoopController.
	// --- concurrency control
	parallelism       int                        // parallelism determines how many goroutines to keep in executor.
	registerChannel   chan resourceAccessRequest // A pipeline used to register gameObjects to the pool. When calling Create from SDK, load balancing is applied to distribute a create request to this channel.
	unregisterChannel chan resourceAccessRequest // A pipeline used to unregister gameObjects to the pool When calling Destroy from SDK, load balancing is applied to distribute a destroy request to this channel.
	// --- timing control
	startTime      time.Time
	physicalFPS    time.Duration // Physical update rate.
	renderFPS      time.Duration // Render update rate.
	renderTicker   *time.Ticker  // Render update ticker.
	physicalTicker *time.Ticker  // Physical update ticker.
	// --- synchronization
	executor *cc.Executor    // executor is a goroutine pool.
	wg       *sync.WaitGroup // wg is used for Wait() method to continue after all loops stoppped.
	sigKill  chan struct{}
	running  bool
}

// NewApplication returns a new masterGameLoopController.
// Not thread safe, no need to do that.
func NewApplication(cfg *AppConfig) *Application {
	if !atomic.CompareAndSwapInt32(&casList[Cas_CoreController], Cas_False, Cas_True) {
		panic("cannot have two masterGameLoopController in a standalone process")
	}
	if cfg.Parallelism < 1 {
		cfg.Parallelism = 1
	}
	app = &Application{
		initFunc:          cfg.InitFunc,
		status:            GameLoopStats_Initialized,
		parallelism:       cfg.Parallelism,
		registerChannel:   make(chan resourceAccessRequest, InstantiateChannelSize),
		unregisterChannel: make(chan resourceAccessRequest, DeconstructionChannelSize),
		physicalFPS:       time.Duration(cfg.PhysicalFps),
		renderFPS:         time.Duration(cfg.RenderFps),
		renderTicker:      time.NewTicker(time.Second / time.Duration(cfg.RenderFps)),
		physicalTicker:    time.NewTicker(time.Second / time.Duration(cfg.PhysicalFps)),
		executor:          cc.NewExecutor(cfg.Parallelism),
		wg:                &sync.WaitGroup{},
		sigKill:           make(chan struct{}, 1),
		running:           false,
	}

	graphics.SetScreenResolution(cfg.Resolution.X, cfg.Resolution.Y)

	return app
}

// Start creates goroutine for each subGameLoopController to work. Not blocking.
// Not thread safe, you have no need, and should not call Start in concurrent execution environment.
func (app *Application) Start() {
	if app.status == GameLoopStats_Running {
		panic("cannot run a controller twice")
	}

	window := InitOpenGL(graphics.GetScreenResolution(), title)
	app.initFunc()

	app.executor.Run()
	go app.runWorkerLoop()

	app.running = true
	app.status = GameLoopStats_Running

	// bootup executor
	app.executor.Run()

	// --- begin render infinite loop
	app.wg.Add(1)
	RenderLoop(window, app.doRender, app.sigKill)
	app.wg.Done()
	// --- infinite loop has stopped, maybe sigkill or something else
}

// Kill terminates all sub workers.
func (g *Application) Kill() {
	fmt.Println("kill")
	g.sigKill <- struct{}{} // kill openGL routine (main routine) (may panic if the channel has been closed)
	g.running = false
}

// Wait MasterLoop and all subLoops to be killed. Blocking.
func (g *Application) Wait() {
	g.wg.Wait()
}

//____________________________________
//
// 		 WorkerLoopController
//____________________________________

func (app *Application) runWorkerLoop() {
	app.startTime = time.Now()
	// before run, enable all systems
	for _, system := range name2System {
		system.GetSystemBase().Enable()
	}

	for app.running {
		select {
		case <-app.physicalTicker.C:
			app.doPhysicalUpdate()
		case <-app.sigKill:
			app.workLoopExit()
			return
		}
	}
	app.workLoopExit()
}

func (app *Application) workLoopExit() {
	close(app.sigKill)
	fmt.Println("sub: wg --")
	app.wg.Done()
}

//____________________________________
//
// 		  Processor Functions
//____________________________________

func (g *Application) doRender() {

	renderSortList = renderSortList[:0]

	mutexList[Mutex_ActivePool].RLock()
	for _, pool := range activePool {
		for elem := range pool {
			renderSortList = append(renderSortList, elem.GetGameObject2D())
		}
	}
	mutexList[Mutex_ActivePool].RUnlock()

	// sort by z from far to near
	sort.Slice(renderSortList, func(i, j int) bool {
		return renderSortList[i].Sprite.Z > renderSortList[j].Sprite.Z
	})
	for _, elem := range renderSortList {
		elem.Callbacks.OnRender(elem.GetIGameObject2D())
	}
}

func (g *Application) doPhysicalUpdate() {
	// 1. check whether there are items to create
	for len(g.registerChannel) > 0 {
		req := <-g.registerChannel
		addObjDefault(req.payload, *req.isActive)
	}
	// 2. execute ECS-system
	for _, sys := range systemPriorityList {
		if sys.GetSystemBase().IsEnabled() {
			sys.Execute(app.executor)
		}
	}
	// 3. do user steps
	for _, pool := range activePool {
		for iobj2d, _ := range pool {
			iobj2d.GetGameObject2D().Callbacks.OnStep(iobj2d)
			iobj2d.GetGameObject2D().Sprite.DoFrameStep()
		}
	}
	// 4. flush input buffer, only one subLoop can do this.
	FlushInputBuffer()
	// 5. check whether there are items to unregister
	for len(g.unregisterChannel) > 0 {
		req := <-g.unregisterChannel
		removeObjDefault(req.payload, req.payload.GetGameObject2D().IsActive)
	}
	// 6. memorize current step
	for _, pool := range activePool {
		for iobj2d, _ := range pool {
			tf := iobj2d.GetGameObject2D().GetComponent(component.NameTransform2D).(*component.Transform2D)
			tf.MemXY()
		}
	}
}
